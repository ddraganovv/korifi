package main

import (
	"net/http"
	"net/url"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
)

const (
	OrgQuotasPath   = "/v3/organization_quotas"
	OrgQuotaPath    = "/v3/organization_quotas/{guid}"
	QuotaToOrgsPath = "/v3/organization_quotas/{guid}/relationships/organizations"
)

type OrgQuotas struct {
	serverURL        url.URL
	requestValidator handlers.RequestValidator
}

func NewPluginHandler(serverURL url.URL, requestValidator handlers.RequestValidator) any { //*OrgQuotas {
	return &OrgQuotas{
		serverURL:        serverURL,
		requestValidator: requestValidator,
	}
}

func (h *OrgQuotas) create(r *http.Request) (*routing.Response, error) {
	// authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.org.quotas.create")

	var payload payloads.OrgQuotaCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	orgQuotaRecord := map[string]string{
		"name": "org-quota-name",
		"guid": uuid.NewString(),
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForOrgQuota(orgQuotaRecord)), nil
}

func (h *OrgQuotas) get(r *http.Request) (*routing.Response, error) {
	// authInfo, _ := authorization.InfoFromContext(r.Context())
	// logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.get")

	orgQuotaGUID := routing.URLParam(r, "guid")
	orgQuotaRecord := map[string]string{
		"name": "org-quota-name",
		"guid": orgQuotaGUID,
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForOrgQuota(orgQuotaRecord)), nil
}

func (h *OrgQuotas) assign(r *http.Request) (*routing.Response, error) {
	// authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.get")

	_ = routing.URLParam(r, "guid")
	var payload payloads.OrgQuotaAssign
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	orgQuotaRecord := map[string]any{
		"data": payload.Data,
	}

	return routing.NewResponse(http.StatusOK).WithBody(orgQuotaRecord), nil
}

func (h *OrgQuotas) update(r *http.Request) (*routing.Response, error) {
	// authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.domain.get")

	var payload payloads.OrgQuotaCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	orgQuotaRecord := map[string]string{
		"name": "org-quota-name",
		"guid": payload.GUID,
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForOrgQuota(orgQuotaRecord)), nil
}

func (h *OrgQuotas) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *OrgQuotas) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: OrgQuotasPath, Handler: h.create},
		{Method: "GET", Pattern: OrgQuotaPath, Handler: h.get},
		{Method: "POST", Pattern: QuotaToOrgsPath, Handler: h.assign},
		{Method: "PATCH", Pattern: OrgQuotaPath, Handler: h.update},
	}
}
