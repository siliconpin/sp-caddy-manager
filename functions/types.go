package functions

type DomainRequest struct {
	Action      string `json:"action"`
	Domain      string `json:"domain"`
	Port        int    `json:"port"`
	Content     string `json:"content"`
	BackendHost string `json:"backend_host"`
}
