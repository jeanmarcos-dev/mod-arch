// Brazilian Portuguese translations of every glossary entry. Keyed by the
// canonical Spanish `term` from glossary.ts so the lookup is a single
// Map.get when the active locale is 'pt'. Identifiers (PL, EAX, IRQ, hex
// vectors, syscall names) stay in their canonical x86/Linux form regardless
// of locale.

export const GLOSSARY_PT: Record<string, { short: string; detail: string }> = {
  // ─── Modes ──────────────────────────────────────────────────────────────
  'PL — Privilege Level': {
    short: 'Nível de privilégio da CPU. PL3 = user, PL0 = kernel.',
    detail: 'O x86 define 4 níveis de privilégio (0-3). Por convenção PL0 = kernel (todas as permissões) e PL3 = user (acesso restrito). Os níveis intermediários quase nunca são usados. O HW verifica o PL em cada acesso de memória, em cada execução de instrução privilegiada, etc. O SW NÃO pode mudar o PL diretamente — apenas o HW o faz em INT/IRET/exceções.',
  },
  'RPL — Requested Privilege Level': {
    short: 'Bits 0-1 de um seletor de segmento. Indicam o privilégio "solicitado" por esse seletor.',
    detail: 'Cada seletor de 16 bits (CS, DS, SS, …) carrega seu RPL codificado nos dois bits menos significativos. Em user mode os seletores têm RPL=3 (ex. CS = 0x1B = 0x18 | 3); em kernel têm RPL=0 (ex. CS = 0x08). A CPU consulta o RPL no IRET para decidir se rebaixa o privilégio: se o CS popped tiver RPL=3, também popa SS e ESP da stack e troca para user mode (inter-privilege IRET). O RPL não deve ser confundido com o CPL (Current Privilege Level), que é o RPL do CS atualmente em uso — o CPL é o "modo real" em que a CPU está rodando; o RPL é o que cada seletor solicita quando carregado.',
  },
  'USER MODE (PL3)': {
    short: 'Processos normais. Acesso restrito.',
    detail: 'Programas de aplicação rodam em PL3. Não podem tocar memória do kernel, executar HLT, IN/OUT, modificar IDT/GDT, etc. Se tentarem, o HW levanta uma exceção que o kernel trata.',
  },
  'KERNEL MODE (PL0)': {
    short: 'Privilégio total. Acesso a tudo.',
    detail: 'O código do SO roda em PL0. Pode ler/escrever qualquer memória, executar qualquer instrução, configurar HW. Entra-se a partir do user via INT (syscall) ou automaticamente quando chega uma IRQ/exceção.',
  },
  'Mode switch': {
    short: 'Transição entre user e kernel. Feita pelo HW, nunca pelo SW.',
    detail: 'Quando chega uma IRQ ou o user executa `int N`, o HW salva o estado (EFLAGS/CS/EIP) e troca de modo automaticamente. O kernel apenas configura QUAL handler invocar (via IDT); a mudança de modo é função do HW. No IRET, o HW desempilha o frame e restaura o modo original.',
  },

  // ─── Registers ──────────────────────────────────────────────────────────
  'EAX, EBX, ECX, EDX': {
    short: 'Registradores de propósito geral de 32 bits.',
    detail: 'GP = General Purpose. EAX (Accumulator), EBX (Base), ECX (Counter), EDX (Data). Por convenção Linux i386, as syscalls usam: EAX = número, EBX = arg1, ECX = arg2, EDX = arg3, EAX = resultado.',
  },
  'ESI, EDI': {
    short: 'Source Index / Destination Index.',
    detail: 'Historicamente usados por instruções de string (MOVS, LODS, STOS) copiando bytes de [ESI] para [EDI]. Hoje são utilizados como registradores de propósito geral.',
  },
  'ESP — Stack Pointer': {
    short: 'Aponta para o topo atual da pilha (o último slot ocupado).',
    detail: 'Cada PUSH decrementa em 4 bytes e escreve o valor; cada POP lê e incrementa em 4. CALL/RET e INT/IRET também o modificam automaticamente: CALL empilha o endereço de retorno (RET o desempilha); INT empilha EFLAGS/CS/EIP (IRET os desempilha). A pilha cresce PARA BAIXO — de endereços altos para baixos — portanto ESP diminui à medida que o trabalho se acumula. Importante: o valor concreto de ESP depende do modo. Em PL3 aponta para a pilha do processo de usuário; em PL0 aponta para a kernel stack (a CPU recarrega ESP automaticamente em uma mudança de privilégio).',
  },
  'EBP — Base Pointer': {
    short: 'Aponta para a base do stack frame atual. Uma âncora fixa durante uma função.',
    detail: 'Diferentemente do ESP, o EBP não se move a cada PUSH/POP: permanece fixo durante toda a execução da função para que o compilador possa gerar acessos previsíveis a parâmetros e locais. A convenção clássica é o prólogo `PUSH EBP; MOV EBP, ESP` na entrada e `POP EBP; RET` na saída. Dentro da função, os argumentos vivem em `[EBP+8]`, `[EBP+12]`, etc. (após o EBP salvo e o endereço de retorno), e os locais em `[EBP-4]`, `[EBP-8]`, etc. Em resumo: ESP se move constantemente; EBP fica parado.',
  },
  EIP: {
    short: 'Instruction Pointer. A próxima instrução a executar.',
    detail: 'Após buscar os bytes de uma instrução, o EIP aponta para a próxima. JMP/CALL/RET/INT/IRET o modificam explicitamente. Ninguém escreve o EIP diretamente com MOV — a ISA proíbe isso.',
  },
  EFLAGS: {
    short: 'Registrador de flags (status e configuração).',
    detail: 'Cada bit é um flag distinto: ZF (Zero), SF (Sign), CF (Carry), OF (Overflow), IF (Interrupt enable), etc. Instruções aritméticas/CMP atualizam os flags; saltos condicionais (JE/JNE/JG/JL) os leem.',
  },
  'CS, DS, SS': {
    short: 'Seletores de segmento (Code / Data / Stack).',
    detail: 'Seletores de 16 bits que indexam entradas na GDT (ou LDT quando TI=1; este simulador não usa LDT). Os bits 0-1 são o RPL (Requested Privilege Level). Em user mode CS = 0x1B (0x18 | 3); em kernel CS = 0x08. O IRET lê o CS que vai desempilhar para decidir se retorna ao user ou permanece em kernel.',
  },
  'CR2 — Control Register 2': {
    short: 'Endereço linear que disparou o último page fault.',
    detail: 'Quando a MMU levanta um page fault (vetor 0x0E), o HW carrega CR2 com o endereço virtual que o processo tentou tocar. O kernel lê CR2 dentro do handler para decidir o que fazer: resolver a página se a falha é legítima (demand paging, copy-on-write) ou matar o processo se o acesso é inválido. Neste simulador o CR2 aparece no painel de eventos e no payload do evento page_fault sob o nome canônico `cr2` (também disponível como `addr`).',
  },

  // ─── Flags ──────────────────────────────────────────────────────────────
  'ZF — Zero Flag': {
    short: '1 quando o último resultado foi 0.',
    detail: 'Atualizado por ADD/SUB/CMP/AND/etc. Lido por JE/JZ (salta se ZF=1) e JNE/JNZ (salta se ZF=0). CMP é um SUB que só atualiza flags sem gravar o resultado.',
  },
  'SF — Sign Flag': {
    short: '1 quando o último resultado foi negativo.',
    detail: 'Igual ao bit mais significativo (bit 31) do resultado. Lido por JG, JL, JGE, JLE em comparações com sinal.',
  },
  'CF — Carry Flag': {
    short: '1 quando houve overflow sem sinal.',
    detail: 'Por exemplo, somar 0xFFFFFFFF + 1 ativa CF (faz wrap). Usado por instruções que estendem aritmética para 64 bits (ADC, SBB).',
  },
  'OF — Overflow Flag': {
    short: '1 quando houve overflow com sinal.',
    detail: 'Distinto do CF: detecta overflow EM COMPLEMENTO DE DOIS. Ex. 0x7FFFFFFF + 1 ativa OF (vai do máximo positivo ao mínimo negativo).',
  },
  'IF — Interrupt Flag': {
    short: 'Se IF=1, IRQs de HW podem interromper. Se IF=0, são ignoradas.',
    detail: 'Em user mode IF normalmente é 1 para que o timer possa fazer preempção. Quando chega uma IRQ, o HW empilha EFLAGS e zera IF para evitar reentrância. O IRET restaura o EFLAGS desempilhado (reabilitando IF se era 1). HLT com IF=1 aguarda a próxima IRQ. Detalhe fino: no x86 real esse clear de IF depende do tipo de "gate" do IDT — interrupt gates limpam IF, trap gates não. Neste simulador toda entrada é tratada como interrupt gate.',
  },

  // ─── Interrupts ─────────────────────────────────────────────────────────
  Vector: {
    short: 'Índice 0-255 no IDT. Cobre IRQs de HW, exceções da CPU e interrupções de software.',
    detail: 'Qualquer coisa que entre pelo IDT carrega um vetor que indexa o handler correspondente. A taxonomia técnica distingue três origens para esse vetor: IRQ (hardware externo), exceção (a própria CPU ao executar uma instrução) e software interrupt (a instrução INT N). Coloquialmente — e no código deste simulador — "IRQ" é usado como termo guarda-chuva para os três; o glossário, ao contrário, mantém a distinção para que você aprenda a terminologia x86 real. Convenção de nomes nas próximas entradas: prefixo "Vector NN" para exceções da CPU (0x00, 0x0D, 0x0E), prefixo "IRQ NN" para interrupções de hardware externas (0x20, 0x21) e prefixo "INT NN" para interrupções de software (0x80, 0x81). O número do vetor é o mesmo conceito nos três casos: o que muda é a origem do disparo.',
  },
  'IRQ — Interrupt Request': {
    short: 'Sinal externo para a CPU pedindo atenção.',
    detail: 'Em sentido estrito, uma IRQ é um evento assíncrono enviado à CPU por HW externo (timer, teclado, disco). A CPU a atende entre instruções se IF=1. Cada IRQ tem um vetor (número 0-255) que indexa o IDT. Exceções (originadas na própria CPU) e software interrupts (INT N) compartilham o mesmo dispatch, mas NÃO são IRQs em sentido estrito.',
  },
  'Excepción': {
    short: 'Evento disparado pela própria CPU ao executar uma instrução problemática.',
    detail: 'O HW externo não a dispara: a CPU sim, quando uma instrução não pode ser concluída (divisão por zero, acesso inválido a memória, instrução privilegiada em user mode, etc.). Subclassifica-se em fault (EIP aponta para a instrução ofensora, retentável), trap (EIP aponta para a seguinte) e abort (irrecuperável). Entra pelo IDT como uma IRQ, mas tecnicamente NÃO é uma IRQ.',
  },
  'Software interrupt': {
    short: 'Trap disparado pela instrução INT N a partir do código.',
    detail: 'O user ou o kernel executa `INT N`. O HW lê IDT[N], empilha EFLAGS/CS/EIP, troca para kernel mode e salta para o handler. Funcionalmente idêntico ao dispatch de uma IRQ de HW; a diferença é a origem (`source = SW` vs `HW`). Classicamente usado para syscalls (o Linux i386 usa INT 0x80). NÃO é uma IRQ em sentido estrito, ainda que compartilhe o mecanismo.',
  },
  'Vector 0x00 — Divide error': {
    short: 'Exceção quando uma divisão inteira divide por zero (ou estoura).',
    detail: 'Disparada pela própria CPU quando DIV ou IDIV roda com divisor zero, ou quando o quociente não cabe no registrador de destino. Exceção do tipo fault: EIP aponta para a instrução ofensora, não para a seguinte. O kernel normalmente termina o processo ofensor.',
  },
  'IRQ 0x20 — Timer': {
    short: 'Tick periódico do timer programável (PIT).',
    detail: 'A cada `quantum` ticks de CPU essa IRQ de HW dispara. O scheduler a usa para fazer preempção: salvar o PCB atual, escolher o próximo processo, restaurar seu PCB. É isso que torna o SO multitarefa preemptivo. Diferentemente dos vetores 0x00 / 0x0E / 0x80 / 0x81, este SIM é uma IRQ em sentido estrito: HW externo (PIT), assíncrono.',
  },
  'IRQ 0x21 — Keyboard': {
    short: 'Uma tecla foi pressionada.',
    detail: 'O controlador do teclado deposita o scancode/byte em um buffer e dispara essa IRQ de HW. O handler do kernel lê o byte e, se houver um processo BLOCKED aguardando o teclado, entrega o byte e o acorda. Como o timer, uma IRQ em sentido estrito (HW externo, assíncrono).',
  },
  KeyboardBuf: {
    short: 'Fila FIFO interna do kernel que guarda bytes ainda não entregues a um leitor.',
    detail: `Quando você pressiona uma tecla, o handler da IRQ 0x21 anexa o byte ao KeyboardBuf. A partir daí dois cenários podem ocorrer:

• **Entregue** — Se houver um processo BLOCKED em sys_read(stdin), o kernel copia o primeiro byte diretamente para o buffer desse processo e o coloca em READY. O narrador do teclado rotula isto como "PID N (prog): BLOCKED → READY".

• **Bufferizado** — Se ninguém estiver lendo, o byte permanece no KeyboardBuf até que algum processo chame sys_read. O narrador o mostra como "KBuf push · sem leitor (fila: N)".

O badge do console mostra "KBuf {used}/{cap}" com cap = 16, que é um teto puramente didático — espelha a capacidade típica de um buffer de teclado real. Quando "used" excede "cap" o badge fica vermelho e ganha ⚠: é um alerta visual, não perda de dados. O slice subjacente continua crescendo e entrega todos os bytes em ordem FIFO. O ponto pedagógico é que pressionar teclas sem um leitor tem custo (a fila cresce), e que sys_read é o que a esvazia.`,
  },
  'INT 0x80 — Syscall': {
    short: 'Interrupção de software: o user solicita um serviço ao kernel (convenção Linux i386).',
    detail: 'Não é uma IRQ de HW: é uma instrução `int 0x80` que o user executa. O HW troca para kernel mode (como em qualquer INT), e o kernel faz o dispatch por EAX para a syscall correspondente. O Linux moderno usa `sysenter`/`syscall` por desempenho, mas `int 0x80` ainda funciona.',
  },
  'INT 0x81 — Mutex/Semáforo': {
    short: 'Interrupção de software específica do simulador para operações de mutex e semáforo.',
    detail: 'Não é Linux padrão — uma convenção deste simulador. EAX escolhe a operação (mutex_lock=1, mutex_unlock=2, sem_wait=3, sem_signal=4); EBX carrega o ID do recurso. O handler vive no kernel e manipula as listas de mutex e semáforos que você vê em /concurrencia.',
  },
  'Vector 0x0E — Page Fault': {
    short: 'Vetor da exceção de page fault, levantada pela MMU.',
    detail: 'O slot do IDT onde a MMU despacha qualquer acesso de memória inválido. Como toda exceção, é levantada pela própria CPU (não por HW externo): quando a unidade de memória detecta que o acesso não pode ser atendido no modo atual, recarrega ESP para a kernel stack, empilha o frame e salta para IDT[0x0E]. Para as condições que a disparam e o que o kernel faz em seguida, veja a entrada "Page fault" na seção de Memória.',
  },
  'Vector 0x0D — General Protection': {
    short: 'Exceção #GP: o user tentou algo que seu nível de privilégio não permite.',
    detail: 'Disparada pela CPU quando um processo em PL3 executa uma instrução privilegiada (HLT, IN/OUT, LGDT/LIDT, MOV CRn, etc.) ou usa um seletor inválido. O kernel termina o processo ofensor com motivo "general_protection". Neste simulador, a causa mais comum que você verá é HLT a partir do user mode — lembre-se de que HLT só é válido em PL0.',
  },
  'INT N': {
    short: 'Instrução que dispara uma interrupção de software.',
    detail: 'O user executa `int N`. O HW lê IDT[N], empilha EFLAGS/CS/EIP, troca para kernel mode, salta para o handler. Funcionalmente IDÊNTICO ao dispatch de uma IRQ de HW; a diferença é apenas a origem (SW vs HW).',
  },
  IRET: {
    short: 'Retorno de interrupção. Desfaz o que INT/dispatch fez.',
    detail: 'Desempilha da stack, NESTA ORDEM DE POP: primeiro EIP (porque está no topo), depois CS, depois EFLAGS. Se o CS desempilhado for user (RPL=3), também desempilha ESP e SS e troca para user mode (isso se chama "inter-privilege IRET"). É o único modo "limpo" de retornar do kernel para o user. A memória física do frame, lida de endereços baixos para altos, é [EIP][CS][EFLAGS][ESP_user][SS_user]: o inverso da ordem de pop porque a pilha cresce para baixo.',
  },
  'IDT — Interrupt Descriptor Table': {
    short: 'Tabela com 256 entradas: vetor → handler.',
    detail: 'Vive na memória do kernel. Cada entrada aponta para o handler do vetor correspondente — independentemente de esse vetor ser disparado por uma IRQ de HW, uma exceção da CPU ou um INT N. O HW a consulta automaticamente no dispatch. No x86 real cada entrada é um "gate descriptor" de 8 bytes; em nosso simulador simplificamos para ponteiros de 4 bytes.',
  },

  // ─── Processes ──────────────────────────────────────────────────────────
  'PCB — Process Control Block': {
    short: 'Estrutura do kernel que guarda o estado de um processo.',
    detail: 'Para cada processo o kernel mantém um PCB com: PID, nome, estado, snapshot de TODOS os registradores, endereço do código, endereço da pilha, lista de arquivos abertos, prioridade, etc. No Linux isto se chama `task_struct`.',
  },
  'PID — Process ID': {
    short: 'Identificador único do processo.',
    detail: 'O kernel atribui um PID monotonicamente ao criar cada processo. PID 1 é tipicamente o init. Em nosso simulador, PID 1 = primeiro processo do demo, PID 2 = segundo, etc.',
  },
  RUN: {
    short: 'Processo atualmente em execução na CPU.',
    detail: 'Apenas UM processo por vez pode estar em RUN (em single-core). Seus registradores estão ao vivo na CPU. Quando o timer dispara, transita para READY.',
  },
  READY: {
    short: 'Pronto para executar, aguardando sua vez na fila de prontos.',
    detail: 'O scheduler o escolherá na próxima preempção. Seus registradores vivem no PCB.',
  },
  BLOCKED: {
    short: 'Aguardando um evento (I/O, sleep, mutex).',
    detail: 'NÃO consome CPU. Permanece na fila de espera. Quando o evento aguardado acontece (por exemplo, uma IRQ de teclado), o kernel o move para READY.',
  },
  ZOMBIE: {
    short: 'Processo terminou, mas o PCB ainda existe.',
    detail: 'Quando um processo chama sys_exit (ou morre por exceção), o kernel marca seu PCB como ZOMBIE. O PCB permanece para que o pai possa ler o exit code com wait(). Nosso demo o mantém por simplicidade.',
  },
  'Context switch': {
    short: 'Mudança do processo que está em RUN.',
    detail: 'Quando ocorre: 1) Salva o estado do processo atual em seu PCB. 2) Escolhe o próximo. 3) Carrega seu PCB na CPU. 4) Continua. Em nosso simulador isso acontece entre o dispatch da IRQ e o IRET — o frame na kernel stack é "patcheado" com os registradores do novo processo.',
  },
  'Ready queue': {
    short: 'Fila FIFO de processos READY.',
    detail: 'O scheduler RR a utiliza: tira da cabeça para executar, empurra na cauda o que perde a CPU. Outros schedulers (CFS, prioridades) usam estruturas mais complexas.',
  },
  'Waiting queue': {
    short: 'Fila de processos BLOCKED aguardando um recurso.',
    detail: 'Nós a renderizamos como uma única lista por simplicidade. Em kernels reais existe UMA fila de espera POR recurso: uma por dispositivo de I/O (teclado, disco, cada socket de rede), uma por mutex, uma por semáforo, uma por fila de mensagens. Assim, quando um evento chega — por exemplo a IRQ do teclado — o kernel sabe exatamente qual processo acordar inspecionando APENAS a fila desse recurso, sem varrer a lista global de processos. A separação também permite políticas por dispositivo: a fila de disco pode ser FCFS, a de socket pode ser baseada em prioridade, etc.',
  },
  'TID — Thread ID': {
    short: 'Identificador único de uma tarefa, seja ela um processo ou uma thread.',
    detail: 'No Linux tudo é uma task: o que chamamos de "processo" tem seu próprio TID, e as threads do mesmo processo compartilham um TGID mas cada uma mantém seu próprio TID. Neste simulador o TID coincide com o PID que o kernel atribui a cada task; o alias semântico ajuda a distinguir "sou uma thread" de "sou um processo" quando ambos compartilham um espaço de endereços.',
  },
  'TGID — Thread Group ID': {
    short: 'PID do líder do grupo de threads. Todas as threads do mesmo "processo" compartilham um TGID.',
    detail: 'fork() cria um novo TGID (= PID do filho). clone(CLONE_THREAD) herda o TGID do pai, portanto getpid() retorna o mesmo valor para todas as threads do processo, enquanto gettid() retorna o TID real. Na UI agrupamos cards por TGID para que cada "processo" apareça como um nó com suas threads aninhadas embaixo.',
  },
  'clone()': {
    short: 'Syscall primitiva do Linux para criar tasks com flags de compartilhamento.',
    detail: 'fork(), vfork() e pthread_create() são casos particulares de clone(). As flags relevantes aqui: CLONE_VM (compartilha espaço de endereços), CLONE_FILES (compartilha tabela de fds), CLONE_THREAD (herda o TGID, vira uma thread irmã), CLONE_VFORK (bloqueia o pai até o filho terminar). Combiná-las produz o comportamento clássico do pthread_create: CLONE_VM | CLONE_FILES | CLONE_THREAD.',
  },
  pthread_create: {
    short: 'API POSIX para criar uma thread. Na glibc traduz para clone().',
    detail: 'pthread_create(thread, attr, fn, arg) pede ao kernel uma task que compartilhe memória e fds com o chamador, e inicie em fn(arg). Neste simulador é um wrapper sobre sys_clone(120) com as flags CLONE_VM|CLONE_FILES|CLONE_THREAD; pthread_join é modelado com waitpid no TID.',
  },
  'Race condition': {
    short: 'Bug em que duas tasks competem por um dado sem sincronização e o resultado depende da intercalação.',
    detail: 'O demo threads_race o mostra: duas threads fazem counter++, que na verdade é load-inc-store. Se o timer interrompe entre load e store, a outra thread pode ler o mesmo valor desatualizado e ambas acabam gravando o mesmo número — um incremento é perdido. Com quantum pequeno e 200 iterações, o counter final fica bem abaixo de 400. A correção didática é threads_safe: envolver a seção crítica com mutex_lock/unlock.',
  },

  // ─── Scheduler ──────────────────────────────────────────────────────────
  Quantum: {
    short: 'Quantos ticks de CPU um processo recebe antes da troca de contexto.',
    detail: 'O knob central do Round-Robin. Quantum pequeno = mais responsividade, mais overhead. Quantum grande = menos overhead, menos responsividade. Em nosso simulador é ajustável ao vivo a partir da topbar.',
  },
  'Round-Robin (RR)': {
    short: 'Cada processo recebe a CPU em turnos iguais.',
    detail: 'O scheduler mais simples e justo. Mantém uma fila circular de prontos. Quando o timer dispara, o processo atual vai para a cauda e o da cabeça vira RUN. Todos os processos recebem o mesmo tempo.',
  },
  Preemption: {
    short: 'O kernel tira a CPU do processo mesmo que ele não quisesse ceder.',
    detail: 'Em multitarefa preemptiva, o timer força a troca. O oposto é "cooperativo", em que o processo precisa ceder voluntariamente — arriscado, um loop infinito trava o sistema.',
  },
  Turnaround: {
    short: 'Tempo total desde que o processo entra até terminar (ZOMBIE).',
    detail: 'TAT = endedAtTick − createdAtTick. Mede a latência total que o usuário percebe para que seu trabalho seja concluído. Um bom scheduler tende a minimizar a média de TAT sem sacrificar a justiça entre processos. O painel exibe "—" enquanto o processo ainda está vivo.',
  },
  'Waiting time': {
    short: 'Soma de ticks que o processo passou na fila READY sem executar.',
    detail: 'WT = totalReadyTicks. Não inclui o tempo em BLOCKED (isso é espera por recurso, não por CPU). Sob Round-Robin, o WT cresce proporcionalmente à quantidade de processos que competem pela CPU. É a métrica mais comum para avaliar justiça entre processos.',
  },
  'Response time': {
    short: 'Ticks entre a criação do processo e a primeira vez que ele recebe a CPU.',
    detail: 'RT = firstRunAtTick − createdAtTick. Importante em sistemas interativos: se um comando demora 200 ms para começar a executar, o usuário o percebe como lento mesmo que o TAT seja bom. Sob Round-Robin, o RT é limitado por (N−1)·quantum, onde N é o número de processos READY.',
  },
  Throughput: {
    short: 'Processos concluídos por unidade de tempo.',
    detail: 'Throughput = processos_ZOMBIE / tempo_decorrido. Aqui expressamos como "procs por 1000 ticks" para manter a escala legível. Cresce quando o scheduler deixa processos curtos terminarem rapidamente; cai quando há muito overhead de context-switch (quantum pequeno demais) ou muitos bloqueios.',
  },

  // ─── Concurrency ────────────────────────────────────────────────────────
  'Critical section': {
    short: 'Seção crítica: um trecho de código que NÃO pode executar em duas threads ao mesmo tempo.',
    detail: 'Se duas threads modificam a mesma variável sem proteger o acesso, os resultados corrompem (race condition). A seção crítica é o bloco que toca o recurso compartilhado; precisa ser protegida por um mecanismo de exclusão mútua (mutex, semáforo, instrução atômica). Mantenha-a o mais curta possível: quanto mais longa, mais contenção.',
  },
  Mutex: {
    short: 'Lock de exclusão mútua: garante que apenas uma thread entre na seção crítica.',
    detail: 'Tem dois estados: livre ou detido por um único holder. A syscall mutex_lock bloqueia a thread se o mutex já estiver detido (envia-a para BLOCKED na fila do mutex); mutex_unlock libera e acorda o primeiro waiter. Neste simulador, cada mutex aparece em /concurrencia com seu holder e sua fila de espera.',
  },
  Semaphore: {
    short: 'Semáforo: contador com operações atômicas de wait (P) e signal (V).',
    detail: 'Generaliza o mutex: o contador permite até N threads concorrentes na seção protegida. wait decrementa o contador (bloqueia a thread se 0); signal incrementa (acorda um waiter, se houver). Útil para limitar acesso a um pool de recursos ou coordenar produtor-consumidor.',
  },
  'Atomic operation': {
    short: 'Operação atômica: o hardware garante que executa sem ser interrompida.',
    detail: 'No x86, instruções como LOCK XADD, LOCK CMPXCHG ou XCHG travam o barramento durante o load-modify-store. São a primitiva sobre a qual mutexes, spinlocks e contadores lock-free são construídos. Sem atomicidade, i++ se decompõe em MOV/INC/MOV — interruptível e portanto inseguro entre threads.',
  },
  Deadlock: {
    short: 'Bloqueio mútuo: duas ou mais threads esperam umas pelas outras em ciclo e nenhuma progride.',
    detail: 'As quatro condições de Coffman devem valer simultaneamente: exclusão mútua, hold-and-wait, sem preempção e espera circular. O demo dining_philosophers reproduz o caso clássico — cada filósofo pega um garfo e espera pelo outro, formando um ciclo. Previne-se quebrando qualquer uma das quatro (por exemplo, pegando garfos em uma ordem global).',
  },
  'Lock contention': {
    short: 'Contenção: muitas threads competem pelo mesmo lock e serializam.',
    detail: 'Cada vez que uma thread encontra o mutex detido, bloqueia e entra na fila — overhead puro. Uma seção crítica curta reduz a contenção; um lock fino por recurso a distribui. Em /concurrencia, a fila de waiters de cada mutex é o termômetro visual de quanta contenção existe.',
  },
  'Producer-consumer': {
    short: 'Padrão produtor-consumidor: uma thread produz dados, outra os processa, compartilhando um buffer.',
    detail: 'O buffer compartilhado precisa de proteção (mutex) e coordenação (dois semáforos: empty e full). O produtor faz wait em empty antes de escrever; o consumidor faz wait em full antes de ler. É a base de pipelines, filas de mensagens e muitas APIs de I/O assíncrono.',
  },
  'Dining philosophers': {
    short: 'Filósofos jantando: exemplo canônico de deadlock com N threads compartilhando N recursos.',
    detail: 'Cinco filósofos compartilham uma mesa com cinco garfos; cada um precisa de dois garfos para comer. Se todos pegam o garfo da esquerda ao mesmo tempo, esperam para sempre pelo da direita (deadlock). Soluções clássicas: ordenar globalmente os garfos, ou limitar a N-1 filósofos comendo simultaneamente. Este simulador tem dois demos separados: `philos_deadlock` (os filósofos travam em ciclo e o sistema deixa de progredir) e `philos_safe` (um deles inverte a ordem de pegar — garfo de menor id primeiro, depois o de maior — e essa assimetria quebra a espera circular).',
  },
  safe_counter: {
    short: 'Demo de incremento protegido por mutex — o contador termina exato.',
    detail: 'Dois workers rodam em paralelo um loop que pega o mutex, incrementa um contador compartilhado e libera. Comparado com race_counter, aqui o resultado final é determinístico (sempre 2 × iterações) porque a seção crítica está protegida.',
  },
  race_counter: {
    short: 'Demo SEM proteção — o contador perde incrementos por race condition.',
    detail: 'Mesmo loop do safe_counter mas sem o mutex. Como load-modify-store não é atômico, dois workers podem ler o mesmo valor antes que o outro escreva, perdendo o incremento. O total final varia entre execuções; o delta para o resultado teórico mede quantos incrementos foram perdidos.',
  },

  // ─── Memory ─────────────────────────────────────────────────────────────
  'MMU — Memory Management Unit': {
    short: 'Hardware da CPU que verifica permissões e traduz endereços.',
    detail: 'Em cada acesso de memória, a MMU consulta tabelas (page tables no x86 real, regiões em nosso simulador) e decide: o modo atual é permitido neste endereço? Se não → page fault. No x86 real ela também traduz endereços virtuais para físicos; simplificamos para memória flat.',
  },
  'GDT — Global Descriptor Table': {
    short: 'Tabela de descritores de segmento.',
    detail: 'No x86 cada segmento tem um descritor (base, limite, permissões). Os seletores (CS/DS/SS) são índices nesta tabela. Em nosso simulador a GDT é nominal — os seletores são usados apenas como pistas visuais, nenhuma tradução real acontece.',
  },
  'TSS — Task State Segment': {
    short: 'Estrutura por core onde o x86 armazena o kernel SS:ESP e outros dados de task-switch.',
    detail: 'No x86 real, quando uma interrupção muda o privilégio de PL3 para PL0, o HW consulta o TSS do core para saber qual endereço recarregar em SS:ESP — esse par (SS0, ESP0) é o topo da kernel stack. O TSS vive em memória, seu endereço é carregado em TR (Task Register) e seu descritor reside na GDT. Neste simulador o TSS não existe como estrutura própria: seus dois campos críticos são simplificados em `KernelStackTop` (o papel de ESP0) e manuseio direto dentro da CPU. Cada core carrega seu próprio `KernelStackTop` para que traps concorrentes não pisem nos frames uns dos outros. Na entrada do kernel a partir do user, o ESP do user é salvo em `SavedUserESP` — o papel que no x86 real o TSS cumpre na troca — e o IRET o restaura ao retornar para PL3.',
  },
  'Page fault': {
    short: 'O acesso à memória é inválido e a MMU o rejeita.',
    detail: 'Causas típicas: o processo PL3 tenta tocar memória do kernel, escreve em uma página somente leitura (como user_code) ou acessa um endereço fora de sua região alocada. Em kernels reais um page fault NÃO é sempre fatal — também é usado para resolução sob demanda: demand paging, copy-on-write após um fork, mapeamento preguiçoso de heap. O kernel decide caso a caso: resolver (mapeando a página) ou matar o processo. Neste simulador todo page fault é fatal: o processo vira ZOMBIE com BlockedOn="page_fault". O dispatch do HW passa pelo vetor 0x0E (veja a entrada em Interrupções).',
  },
  'Error code (page fault)': {
    short: 'Bitmask de 32 bits descrevendo a causa do page fault.',
    detail: 'Segue o layout do Intel SDM Vol 3 §6.15: bit 0 (P) = 1 para violação de permissão, 0 se a página não estava presente; bit 1 (W) = 1 para acesso de escrita, 0 para leitura; bit 2 (U) = 1 se a falha veio de PL3 (user), 0 de PL0 (kernel); bit 4 (I-D) = 1 se foi um instruction fetch. Exemplo: error_code = 7 = 0b111 → bits P, W, U setados → escrita de user em uma página presente mas protegida. No x86 real o HW empilha esse código na stack depois do frame; este simulador o escreve em AddrErrCodeMirror para que o handler o leia com um `mov` e evite layouts assimétricos.',
  },

  // ─── Syscalls ───────────────────────────────────────────────────────────
  Syscall: {
    short: 'Chamada do user ao kernel para solicitar um serviço.',
    detail: 'O user não pode tocar HW diretamente (PL3). Para ler um arquivo, escrever na tela, dar fork em processos, etc., FAZ `int 0x80` (ou `syscall` em x86-64). O kernel responde. A interface é estável entre versões do kernel.',
  },
  'sys_write (4)': {
    short: 'write(fd, buf, count) — escreve bytes em um fd.',
    detail: 'EAX=4, EBX=fd, ECX=buf, EDX=count. fd=1 é stdout. Retorna em EAX o número de bytes escritos ou -1 em erro.',
  },
  'sys_read (3)': {
    short: 'read(fd, buf, count) — lê bytes de um fd.',
    detail: 'EAX=3, EBX=fd, ECX=buf, EDX=count. fd=0 é stdin. Se não houver dados disponíveis, BLOQUEIA o processo até que cheguem (via IRQ do dispositivo).',
  },
  'sys_exit (1)': {
    short: 'exit(status) — termina o processo.',
    detail: 'EAX=1, EBX=status. O processo vira ZOMBIE; se houver outros processos READY, o scheduler lhes dá a CPU. Se era o último, a CPU pára.',
  },

  // ─── Instructions ───────────────────────────────────────────────────────
  HLT: {
    short: 'Halt: pára a CPU até a próxima IRQ.',
    detail: 'Instrução privilegiada (apenas PL0). No loop ocioso do kernel: quando não há processos READY, fazemos HLT com IF=1 — a CPU dorme até que o timer ou outra IRQ a acorde.',
  },
  NOP: {
    short: 'No operation. Queima um ciclo.',
    detail: 'Útil para padding (alinhamento), debugging, ou como placeholder. Em nossos handlers minimalistas usamos `nop iret` para que o dispatch seja visível.',
  },
  'JMP / JE / JNE / JG / JL': {
    short: 'Saltos. Os condicionais leem os flags.',
    detail: 'JMP é incondicional. JE/JZ salta se ZF=1. JNE/JNZ se ZF=0. JG/JL para comparações com sinal (leem SF e OF). Úteis após um CMP para tomar decisões.',
  },
  'CMP a, b': {
    short: 'Compare: faz (a - b) e atualiza flags, sem gravar o resultado.',
    detail: 'Equivalente a SUB mas descarta o resultado. Deixa apenas os flags. Tipicamente seguido por um salto condicional.',
  },
};
