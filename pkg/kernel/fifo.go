package kernel

import (
	"github.com/jeanmarcos-dev/mod-arch/pkg/cpu"
)

// FIFO file flags used by sys_open. Aliased loosely to the Linux constants
// for the demos; only the read/write distinction is honored.
const (
	OpenReadOnly  uint32 = 0
	OpenWriteOnly uint32 = 1
)

// maxFifoNameLen caps how many bytes the kernel will read from the user
// when resolving a FIFO name. Keeps the loop bounded if user code passes
// a non-terminated pointer.
const maxFifoNameLen = 32

// readUserString copies a null-terminated string from user memory up to
// maxFifoNameLen bytes. Returns the string and true on success, "" and
// false if the buffer wanders out of user-accessible memory before a NUL.
func (k *Kernel) readUserString(addr uint32) (string, bool) {
	buf := make([]byte, 0, 16)
	for i := uint32(0); i < maxFifoNameLen; i++ {
		if _, ok := userBufferFault(addr+i, 1); !ok {
			return "", false
		}
		b := k.Mem.Bytes[addr+i]
		if b == 0 {
			return string(buf), true
		}
		buf = append(buf, b)
	}
	return string(buf), true
}

// sysMknod registers a FIFO under the given name in a tiny VFS map. Real
// Linux mknod can create any kind of special file; we only ever target
// FIFOs and ignore the type/mode arguments.
//
//	EBX = name_addr (null-terminated path string in user memory)
//	ECX = type (ignored; assumed S_IFIFO)
//	EDX = mode (ignored)
//
// Returns 0 on success, ^uint32(0) on collision with an existing name
// or on a memory fault.
func (k *Kernel) sysMknod(core *Core, nameAddr uint32) (uint32, []cpu.Event) {
	pcb := k.Processes[core.CurrentPID]
	if pcb == nil {
		return ^uint32(0), nil
	}
	name, ok := k.readUserString(nameAddr)
	if !ok || name == "" {
		return ^uint32(0), nil
	}
	if k.FifoByName == nil {
		k.FifoByName = make(map[string]int)
	}
	if _, exists := k.FifoByName[name]; exists {
		return ^uint32(0), nil
	}

	pipe := &Pipe{
		ID:     len(k.Pipes) + 1,
		Buf:    make([]byte, 0, defaultPipeCap),
		Cap:    defaultPipeCap,
		IsFifo: true,
		Name:   name,
	}
	k.Pipes = append(k.Pipes, pipe)
	k.FifoByName[name] = pipe.ID

	return 0, []cpu.Event{cpu.EventPipeCreate{
		PipeID: pipe.ID, OwnerPID: core.CurrentPID,
		FdR: 0, FdW: 0, IsFifo: true, Name: name,
	}}
}

// sysOpen resolves a FIFO name to a fresh file descriptor in the caller's
// FDTable. The flags argument selects the end:
//
//	flags == OpenWriteOnly → pipe-write fd, Writers++
//	otherwise (default RDONLY) → pipe-read fd, Readers++
//
// The fd number lands in EAX. Returns ^uint32(0) when the name is not
// registered or when user memory cannot be read.
func (k *Kernel) sysOpen(core *Core, nameAddr, flags uint32) (uint32, []cpu.Event) {
	pcb := k.Processes[core.CurrentPID]
	if pcb == nil {
		return ^uint32(0), nil
	}
	name, ok := k.readUserString(nameAddr)
	if !ok || name == "" {
		return ^uint32(0), nil
	}
	pipeID, exists := k.FifoByName[name]
	if !exists {
		return ^uint32(0), nil
	}
	pipe := k.Pipes[pipeID-1]
	var fd uint32
	if flags == OpenWriteOnly {
		fd = allocFD(pcb, "pipe-write", pipe.ID)
		pipe.Writers++
	} else {
		fd = allocFD(pcb, "pipe-read", pipe.ID)
		pipe.Readers++
	}
	return fd, nil
}
