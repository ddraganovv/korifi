package payloads

type Apps struct {
	TotalMemoryInMB      int `json:"total_memory_in_mb"`
	PerProcessMemoryInMB int `json:"per_process_memory_in_mb"`
}

type Services struct {
	PaidServicesAllowed   int `json:"paid_services_allowed"`
	TotalServiceInstances int `json:"total_service_instances"`
}

type Routes struct {
	TotalRoutes int `json:"total_routes"`
}

type OrgQuotaCreate struct {
	GUID     string   `json:"guid"`
	Name     string   `json:"name"`
	Apps     Apps     `json:"apps"`
	Services Services `json:"services"`
	Routes   Routes   `json:"routes"`
}

type Org struct {
	GUID string `json:"guid"`
}
type OrgQuotaAssign struct {
	Data []Org `json:"data"`
}
