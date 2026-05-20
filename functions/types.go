package functions

type DomainRequest struct {
	Action      string `json:"action"`
	Domain      string `json:"domain"`
	Port        int    `json:"port"`
	Content     string `json:"content"`
	BackendHost string `json:"backend_host"`
}

type AuthRequest struct {
	Label    string `json:"label"`
	KeyValue string `json:"key_value"`
}
