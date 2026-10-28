package kernel

import (
	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// defaultPipeCap is the size of a pipe's internal buffer in bytes. 16 is
// big enough for the demos ("HOLA", "ABCDEFGH") yet small enough that
// blocking is observable in the IPC panel after a few writes.
const defaultPipeCap = 16

// Pipe models a one-way byte channel between processes. The buffer is a
// fixed-size FIFO; writers append at the tail, readers consume from the
// head. Readers and Writers count how many fds still reference each end
// across the whole process table — when Writers drops to zero the kernel
// signals EOF to blocked readers; when Readers drops to zero, EPIPE.
type Pipe struct {
	ID      int
	Buf     []byte
	Cap     int
	Readers int
	Writers int
	Name    string // non-empty when this Pipe is a FIFO
	IsFifo  bool
}

// allocFD finds the lowest unused index >= 3 in the caller's FDSpace table,
// grows the table as needed, and stamps the slot with the given Kind +
// PipeID. Returns the fd number.
func allocFD(pcb *PCB, kind string, pipeID int) uint32 {
	fds := pcb.FDSpace()
	if fds == nil {
		return ^uint32(0)
	}
	// Reserve slots 0/1/2 so they stay aligned with stdin/stdout/stderr
	// even though the actual stdio dispatch does not consult the table.
	if len(fds.Table) < 3 {
		fds.Table = append(fds.Table, FD{}, FD{}, FD{})
		fds.Table = fds.Table[:3]
	}
	for i := 3; i < len(fds.Table); i++ {
		if !fds.Table[i].Used {
			fds.Table[i] = FD{Used: true, Kind: kind, PipeID: pipeID}
			return uint32(i)
		}
	}
	fds.Table = append(fds.Table, FD{Used: true, Kind: kind, PipeID: pipeID})
	return uint32(len(fds.Table) - 1)
}

// sysPipe implements sys_pipe(fd_pair_addr): create a new anonymous pipe,
// allocate two fds in the caller's FDTable, and store them at the user
// pointer as two consecutive little-endian dwords (fdR, then fdW).
//
// Returns 0 on success, ^uint32(0) on failure.
func (k *Kernel) sysPipe(core *Core, fdPairAddr uint32) (uint32, []cpu.Event) {
	pcb := k.Processes[core.CurrentPID]
	if pcb == nil {
		return ^uint32(0), nil
	}
	if _, ok := userBufferFault(fdPairAddr, 8); !ok {
		return ^uint32(0), []cpu.Event{cpu.EventPageFault{
			CoreID: core.CPU.CoreID, Addr: fdPairAddr,
			Mode: cpu.ModeUser, PermNeeded: "W",
		}}
	}

	pipe := &Pipe{
		ID:      len(k.Pipes) + 1,
		Buf:     make([]byte, 0, defaultPipeCap),
		Cap:     defaultPipeCap,
		Readers: 1,
		Writers: 1,
	}
	k.Pipes = append(k.Pipes, pipe)

	fdR := allocFD(pcb, "pipe-read", pipe.ID)
	fdW := allocFD(pcb, "pipe-write", pipe.ID)

	k.Mem.Bytes[fdPairAddr+0] = byte(fdR)
	k.Mem.Bytes[fdPairAddr+1] = byte(fdR >> 8)
	k.Mem.Bytes[fdPairAddr+2] = byte(fdR >> 16)
	k.Mem.Bytes[fdPairAddr+3] = byte(fdR >> 24)
	k.Mem.Bytes[fdPairAddr+4] = byte(fdW)
	k.Mem.Bytes[fdPairAddr+5] = byte(fdW >> 8)
	k.Mem.Bytes[fdPairAddr+6] = byte(fdW >> 16)
	k.Mem.Bytes[fdPairAddr+7] = byte(fdW >> 24)

	return 0, []cpu.Event{cpu.EventPipeCreate{
		PipeID: pipe.ID, OwnerPID: core.CurrentPID,
		FdR: fdR, FdW: fdW, IsFifo: false,
	}}
}

// resolvePipeFD looks up fd in the caller's table and returns the Pipe
// object along with a flag for "this fd is the write end" (false = read
// end). Returns nil if the fd is invalid or not a pipe.
func (k *Kernel) resolvePipeFD(pcb *PCB, fd uint32) (*Pipe, bool, bool) {
	fds := pcb.FDSpace()
	if fds == nil {
		return nil, false, false
	}
	if int(fd) >= len(fds.Table) {
		return nil, false, false
	}
	entry := fds.Table[fd]
	if !entry.Used {
		return nil, false, false
	}
	if entry.Kind != "pipe-read" && entry.Kind != "pipe-write" {
		return nil, false, false
	}
	if entry.PipeID < 1 || entry.PipeID > len(k.Pipes) {
		return nil, false, false
	}
	return k.Pipes[entry.PipeID-1], entry.Kind == "pipe-write", true
}

// pipeWrite copies up to count bytes from the caller's buffer into the
// pipe's ring buffer. Stops when the pipe is full or the user buffer is
// exhausted. Returns the number of bytes written and the matching event.
//
// Blocking behavior: if the pipe is full we currently return what was
// written (possibly zero). A future pass adds a real BLOCKED_PIPE_WRITE
// state similar to keyboard reads.
func (k *Kernel) pipeWrite(core *Core, pipe *Pipe, bufAddr, count uint32) (uint32, []cpu.Event) {
	if count == 0 {
		return 0, nil
	}
	if bad, ok := userBufferFault(bufAddr, count); !ok {
		return ^uint32(0), []cpu.Event{cpu.EventPageFault{
			CoreID: core.CPU.CoreID, Addr: bad,
			Mode: cpu.ModeUser, PermNeeded: "R",
		}}
	}

	written := uint32(0)
	captured := make([]byte, 0, count)
	for written < count && len(pipe.Buf) < pipe.Cap {
		b, err := k.Mem.Read8(core.CPU, bufAddr+written, cpu.ModeKernel)
		if err != nil {
			break
		}
		pipe.Buf = append(pipe.Buf, b)
		captured = append(captured, b)
		written++
	}
	if written == 0 {
		return 0, nil
	}
	return written, []cpu.Event{cpu.EventPipeOp{
		PipeID: pipe.ID, PID: core.CurrentPID, Op: "write",
		Bytes: captured, BufLevel: len(pipe.Buf), BufCap: pipe.Cap,
	}}
}

// pipeRead consumes up to count bytes from the head of the pipe's buffer
// into the caller's user buffer. Returns 0 with no event when the buffer
// is empty (callers will eventually block; see pipeWrite TODO).
func (k *Kernel) pipeRead(core *Core, pipe *Pipe, bufAddr, count uint32) (uint32, []cpu.Event) {
	if count == 0 {
		return 0, nil
	}
	if bad, ok := userBufferFault(bufAddr, count); !ok {
		return ^uint32(0), []cpu.Event{cpu.EventPageFault{
			CoreID: core.CPU.CoreID, Addr: bad,
			Mode: cpu.ModeUser, PermNeeded: "W",
		}}
	}

	available := uint32(len(pipe.Buf))
	if available == 0 {
		return 0, nil
	}
	n := count
	if n > available {
		n = available
	}
	bytes := append([]byte(nil), pipe.Buf[:n]...)
	for i := uint32(0); i < n; i++ {
		_ = k.Mem.Write8(core.CPU, bufAddr+i, bytes[i], cpu.ModeKernel)
	}
	pipe.Buf = pipe.Buf[n:]

	return n, []cpu.Event{cpu.EventPipeOp{
		PipeID: pipe.ID, PID: core.CurrentPID, Op: "read",
		Bytes: bytes, BufLevel: len(pipe.Buf), BufCap: pipe.Cap,
	}}
}
