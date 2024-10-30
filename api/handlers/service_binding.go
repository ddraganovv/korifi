package handlers

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
	ServiceBindingsPath = "/v3/service_credential_bindings"
	ServiceBindingPath  = "/v3/service_credential_bindings/{guid}"
)

type ServiceBinding struct {
	appRepo             CFAppRepository
	serviceBindingRepo  CFServiceBindingRepository
	serviceInstanceRepo CFServiceInstanceRepository
	serverURL           url.URL
	requestValidator    RequestValidator
}

//counterfeiter:generate -o fake -fake-name CFServiceBindingRepository . CFServiceBindingRepository
type CFServiceBindingRepository interface {
	CreateServiceBinding(context.Context, authorization.Info, repositories.CreateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
	DeleteServiceBinding(context.Context, authorization.Info, string) error
	ListServiceBindings(context.Context, authorization.Info, repositories.ListServiceBindingsMessage) ([]repositories.ServiceBindingRecord, error)
	GetServiceBinding(context.Context, authorization.Info, string) (repositories.ServiceBindingRecord, error)
	UpdateServiceBinding(context.Context, authorization.Info, repositories.UpdateServiceBindingMessage) (repositories.ServiceBindingRecord, error)
}

func NewServiceBinding(serverURL url.URL, serviceBindingRepo CFServiceBindingRepository, appRepo CFAppRepository, serviceInstanceRepo CFServiceInstanceRepository, requestValidator RequestValidator) *ServiceBinding {
	return &ServiceBinding{
		appRepo:             appRepo,
		serviceInstanceRepo: serviceInstanceRepo,
		serviceBindingRepo:  serviceBindingRepo,
		serverURL:           serverURL,
		requestValidator:    requestValidator,
	}
}

func (h *ServiceBinding) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.create")

	var payload payloads.ServiceBindingCreate
	var serviceBinding repositories.ServiceBindingRecord
	var err error

	if err = h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	if payload.Type == korifiv1alpha1.CFServiceBindingTypeApp {
		serviceBinding, err = h.createTypeApp(r.Context(), authInfo, payload, logger)
		if err != nil {
			return nil, err
		}

	} else {
		serviceBinding, err = h.createTypeKey(r.Context(), authInfo, payload, logger)
		if err != nil {
			return nil, err
		}
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) createTypeApp(ctx context.Context, authInfo authorization.Info, payload payloads.ServiceBindingCreate, logger logr.Logger) (repositories.ServiceBindingRecord, error) {
	app, err := h.appRepo.GetApp(ctx, authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		return repositories.ServiceBindingRecord{}, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get "+repositories.AppResourceType)
	}

	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(ctx, authInfo, payload.Relationships.ServiceInstance.Data.GUID)
	if err != nil {
		return repositories.ServiceBindingRecord{}, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get "+repositories.ServiceInstanceResourceType)
	}

	if app.SpaceGUID != serviceInstance.SpaceGUID {
		return repositories.ServiceBindingRecord{}, apierrors.LogAndReturn(
			logger,
			apierrors.NewUnprocessableEntityError(nil, "The service instance and the app are in different spaces"),
			"App and ServiceInstance in different spaces", "App GUID", app.GUID,
			"ServiceInstance GUID", serviceInstance.GUID,
		)
	}

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(app.SpaceGUID))
	if err != nil {
		return repositories.ServiceBindingRecord{}, apierrors.LogAndReturn(logger, err, "failed to create ServiceBinding", "App GUID", app.GUID, "ServiceInstance GUID", serviceInstance.GUID)
	}

	return serviceBinding, nil
}

func (h *ServiceBinding) createTypeKey(ctx context.Context, authInfo authorization.Info, payload payloads.ServiceBindingCreate, logger logr.Logger) (repositories.ServiceBindingRecord, error) {
	serviceInstance, err := h.serviceInstanceRepo.GetServiceInstance(ctx, authInfo, payload.Relationships.ServiceInstance.Data.GUID)
	if err != nil {
		return repositories.ServiceBindingRecord{}, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "failed to get "+repositories.ServiceInstanceResourceType)
	}

	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(serviceInstance.SpaceGUID))
	if err != nil {
		return repositories.ServiceBindingRecord{}, apierrors.LogAndReturn(logger, err, "failed to create ServiceBinding", "ServiceInstance GUID", serviceInstance.GUID)
	}

	return serviceBinding, nil
}

func (h *ServiceBinding) createUserProvidedServiceBinding(
	ctx context.Context,
	authInfo authorization.Info,
	payload payloads.ServiceBindingCreate,
	app repositories.AppRecord,
) (*routing.Response, error) {
	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(app.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logr.FromContextOrDiscard(ctx), err, "failed to create ServiceBinding")
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) createManagedServiceBinding(
	ctx context.Context,
	authInfo authorization.Info,
	payload payloads.ServiceBindingCreate,
	app repositories.AppRecord,
) (*routing.Response, error) {
	serviceBinding, err := h.serviceBindingRepo.CreateServiceBinding(ctx, authInfo, payload.ToMessage(app.SpaceGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logr.FromContextOrDiscard(ctx), err, "failed to create ServiceBinding")
	}

	return routing.NewResponse(http.StatusAccepted).
		WithHeader("Location", presenter.JobURLForRedirects(serviceBinding.GUID, presenter.ManagedServiceBindingCreateOperation, h.serverURL)), nil
}

func (h *ServiceBinding) delete(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.delete")

	serviceBindingGUID := routing.URLParam(r, "guid")

	err := h.serviceBindingRepo.DeleteServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "error when deleting service binding", "guid", serviceBindingGUID)
	}

	return routing.NewResponse(http.StatusNoContent), nil
}

func (h *ServiceBinding) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.list")

	listFilter := new(payloads.ServiceBindingList)
	if err := h.requestValidator.DecodeAndValidateURLValues(r, listFilter); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	serviceBindingList, err := h.serviceBindingRepo.ListServiceBindings(r.Context(), authInfo, listFilter.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to list "+repositories.ServiceBindingResourceType)
	}

	var appRecords []repositories.AppRecord
	if listFilter.Include != "" && len(serviceBindingList) > 0 {
		listAppsMessage := repositories.ListAppsMessage{}

		for _, serviceBinding := range serviceBindingList {
			listAppsMessage.Guids = append(listAppsMessage.Guids, serviceBinding.AppGUID)
		}

		appRecords, err = h.appRepo.ListApps(r.Context(), authInfo, listAppsMessage)
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "failed to list "+repositories.AppResourceType)
		}
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBindingList(serviceBindingList, appRecords, h.serverURL, *r.URL)), nil
}

func (h *ServiceBinding) update(r *http.Request) (*routing.Response, error) { //nolint:dupl
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.update")

	serviceBindingGUID := routing.URLParam(r, "guid")

	var payload payloads.ServiceBindingUpdate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	_, err := h.serviceBindingRepo.GetServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting service binding in repository")
	}

	serviceBinding, err := h.serviceBindingRepo.UpdateServiceBinding(r.Context(), authInfo, payload.ToMessage(serviceBindingGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error updating service binding in repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.service-binding.get")

	serviceBindingGUID := routing.URLParam(r, "guid")

	serviceBinding, err := h.serviceBindingRepo.GetServiceBinding(r.Context(), authInfo, serviceBindingGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error getting service binding in repository")
	}
	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForServiceBinding(serviceBinding, h.serverURL)), nil
}

func (h *ServiceBinding) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *ServiceBinding) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: ServiceBindingsPath, Handler: h.create},
		{Method: "GET", Pattern: ServiceBindingsPath, Handler: h.list},
		{Method: "DELETE", Pattern: ServiceBindingPath, Handler: h.delete},
		{Method: "PATCH", Pattern: ServiceBindingPath, Handler: h.update},
		{Method: "GET", Pattern: ServiceBindingPath, Handler: h.get},
	}
}
