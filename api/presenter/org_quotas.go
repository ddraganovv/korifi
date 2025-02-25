package presenter

type OrgQuotaResponse struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
}

func ForOrgQuota(record map[string]string) OrgQuotaResponse {
	return OrgQuotaResponse{
		GUID: record["guid"],
		Name: record["name"],
	}
}
