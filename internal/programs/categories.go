package programs

// Category slugs. These strings travel from Demo.Category through the HTTP
// API into the frontend, which uses them as i18n keys and as identifiers in
// admin settings (Demo.Name is the persisted gate key, but Category drives
// grouping). Keep them stable — renames break disabledDemos config and any
// link that references a category.
const (
	CategoryFundamentosCPU    = "fundamentos-cpu"
	CategoryModoDual          = "modo-dual"
	CategoryScheduling        = "scheduling"
	CategoryProcesosIPC       = "procesos-ipc"
	CategoryHilos             = "hilos"
	CategorySincronizacion    = "sincronizacion"
	CategoryFallosExcepciones = "fallos-excepciones"
	CategoryShowcase          = "showcase"
)

// CategoryMeta is the public description of a category, shipped to the
// frontend so it can render labels and ordering without hardcoding them.
type CategoryMeta struct {
	Slug        string `json:"slug"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Order       int    `json:"order"`
}

// Categories returns the catalogue's category list in canonical display
// order. The frontend renders sections in this order; admin tables sort by
// it as well. Labels here are the Spanish fallback used by the backend when
// the request has no locale — the frontend should prefer its own i18n keys.
func Categories() []CategoryMeta {
	return []CategoryMeta{
		{
			Slug:        CategoryFundamentosCPU,
			Label:       "Fundamentos CPU",
			Description: "Registros, stack y control de flujo: el ABC del procesador antes de tocar el sistema operativo.",
			Order:       1,
		},
		{
			Slug:        CategoryModoDual,
			Label:       "Modo dual y syscalls",
			Description: "Cruce user/kernel: int 0x80, mode switch, sys_read/write y la primera transición a PL0.",
			Order:       2,
		},
		{
			Slug:        CategoryScheduling,
			Label:       "Scheduling",
			Description: "Round-robin, timer preemption, quantum y la pelea de varios procesos por la CPU.",
			Order:       3,
		},
		{
			Slug:        CategoryProcesosIPC,
			Label:       "Procesos e IPC",
			Description: "fork, vfork, pipes y waitpid: cómo un proceso engendra otro y se comunican.",
			Order:       4,
		},
		{
			Slug:        CategoryHilos,
			Label:       "Hilos",
			Description: "clone(CLONE_VM|CLONE_THREAD): el modelo de threads sobre el mismo espacio de direcciones.",
			Order:       5,
		},
		{
			Slug:        CategorySincronizacion,
			Label:       "Sincronización",
			Description: "Mutex, semáforos, race conditions y deadlock. Antes/después del lock, lado a lado.",
			Order:       6,
		},
		{
			Slug:        CategoryFallosExcepciones,
			Label:       "Fallos y excepciones",
			Description: "Page fault, stack overflow, syscalls inválidas y operaciones de sync inconsistentes. Cuándo y por qué el kernel mata un proceso.",
			Order:       7,
		},
		{
			Slug:        CategoryShowcase,
			Label:       "Showcase",
			Description: "Recorrido integral del simulador en un solo demo multi-core.",
			Order:       8,
		},
	}
}
