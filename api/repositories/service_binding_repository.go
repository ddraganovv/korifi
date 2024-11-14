package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/webhooks/services/bindings"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelServiceBindingProvisionedService = "servicebinding.io/provisioned-service"
	ServiceBindingResourceType            = "Service Binding"
	ServiceBindingTypeApp                 = "app"
)

type ServiceBindingRepo struct {
	userClientFactory       authorization.UserK8sClientFactory
	namespacePermissions    *authorization.NamespacePermissions
	namespaceRetriever      NamespaceRetriever
	bindingConditionAwaiter Awaiter[*korifiv1alpha1.CFServiceBinding]
}

func NewServiceBindingRepo(
	namespaceRetriever NamespaceRetriever,
	userClientFactory authorization.UserK8sClientFactory,
	namespacePermissions *authorization.NamespacePermissions,
	bindingConditionAwaiter Awaiter[*korifiv1alpha1.CFServiceBinding],
) *ServiceBindingRepo {
	return &ServiceBindingRepo{
		userClientFactory:       userClientFactory,
		namespacePermissions:    namespacePermissions,
		namespaceRetriever:      namespaceRetriever,
		bindingConditionAwaiter: bindingConditionAwaiter,
	}
}

type ServiceBindingRecord struct {
	GUID                string
	Type                string
	Name                *string
	AppGUID             string
	ServiceInstanceGUID string
	SpaceGUID           string
	Labels              map[string]string
	Annotations         map[string]string
	CreatedAt           time.Time
	UpdatedAt           *time.Time
	DeletedAt           *time.Time
	LastOperation       ServiceBindingLastOperation
	Parameters          map[string]any
	Ready               bool
}

func (r ServiceBindingRecord) Relationships() map[string]string {
	return map[string]string{
		"app":              r.AppGUID,
		"service_instance": r.ServiceInstanceGUID,
	}
}

type ServiceBindingLastOperation struct {
	Type        string
	State       string
	Description *string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}

type Credentials map[string]any

type ServiceBindingDetailsRecord struct {
	Credentials    Credentials
	SyslogDrainURL *string
	VolumeMounts   []string
}

type CreateServiceBindingMessage struct {
	Name                *string
	ServiceInstanceGUID string
	AppGUID             string
	SpaceGUID           string
	Parameters          map[string]any
}

type DeleteServiceBindingMessage struct {
	GUID string
}

type ListServiceBindingsMessage struct {
	AppGUIDs             []string
	ServiceInstanceGUIDs []string
	LabelSelector        string
	PlanGUIDs            []string
}

func (m *ListServiceBindingsMessage) matches(serviceBinding korifiv1alpha1.CFServiceBinding) bool {
	return tools.EmptyOrContains(m.ServiceInstanceGUIDs, serviceBinding.Spec.Service.Name) &&
		tools.EmptyOrContains(m.AppGUIDs, serviceBinding.Spec.AppRef.Name) &&
		tools.EmptyOrContains(m.PlanGUIDs, serviceBinding.Labels[korifiv1alpha1.PlanGUIDLabelKey])
}

func (m CreateServiceBindingMessage) toCFServiceBinding() (*korifiv1alpha1.CFServiceBinding, error) {
	guid := uuid.NewString()

	parameterBytes, err := json.Marshal(m.Parameters)
	if err != nil {
		return &korifiv1alpha1.CFServiceBinding{}, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	return &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: m.SpaceGUID,
			Labels:    map[string]string{LabelServiceBindingProvisionedService: "true"},
		},
		Spec: korifiv1alpha1.CFServiceBindingSpec{
			DisplayName: m.Name,
			Service: corev1.ObjectReference{
				Kind:       "CFServiceInstance",
				APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				Name:       m.ServiceInstanceGUID,
			},
			AppRef: corev1.LocalObjectReference{Name: m.AppGUID},
			Parameters: &runtime.RawExtension{
				Raw: parameterBytes,
			},
		},
	}, nil
}

type UpdateServiceBindingMessage struct {
	GUID          string
	MetadataPatch MetadataPatch
}

func (r *ServiceBindingRepo) CreateServiceBinding(ctx context.Context, authInfo authorization.Info, message CreateServiceBindingMessage) (ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfServiceBinding, err := message.toCFServiceBinding()
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to convert to CfServiceBinding: %w", err)
	}

	cfApp := new(korifiv1alpha1.CFApp)
	err = userClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.AppRef.Name, Namespace: cfServiceBinding.Namespace}, cfApp)
	if err != nil {
		return ServiceBindingRecord{},
			apierrors.AsUnprocessableEntity(
				apierrors.FromK8sError(err, ServiceBindingResourceType),
				"Unable to use app. Ensure that the app exists and you have access to it.",
				apierrors.ForbiddenError{},
				apierrors.NotFoundError{},
			)
	}

	err = userClient.Create(ctx, cfServiceBinding)
	if err != nil {
		if validationError, ok := validation.WebhookErrorToValidationError(err); ok {
			if validationError.Type == bindings.ServiceBindingErrorType {
				return ServiceBindingRecord{}, apierrors.NewUniquenessError(err, validationError.GetMessage())
			}
		}

		return ServiceBindingRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	cfServiceInstance := new(korifiv1alpha1.CFServiceInstance)
	err = userClient.Get(ctx, types.NamespacedName{Name: cfServiceBinding.Spec.Service.Name, Namespace: cfServiceBinding.Namespace}, cfServiceInstance)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to get service instance: %w", err)
	}

	if cfServiceInstance.Spec.Type == korifiv1alpha1.UserProvidedType {
		cfServiceBinding, err = r.bindingConditionAwaiter.AwaitCondition(ctx, userClient, cfServiceBinding, korifiv1alpha1.StatusConditionReady)
		if err != nil {
			return ServiceBindingRecord{}, err
		}
	}

	return serviceBindingToRecord(cfServiceBinding)
}

func (r *ServiceBindingRepo) DeleteServiceBinding(ctx context.Context, authInfo authorization.Info, guid string) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	namespace, err := r.namespaceRetriever.NamespaceFor(ctx, guid, ServiceBindingResourceType)
	if err != nil {
		return err
	}

	binding := &korifiv1alpha1.CFServiceBinding{}

	err = userClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: guid}, binding)
	if err != nil {
		return apierrors.ForbiddenAsNotFound(apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	err = userClient.Delete(ctx, binding)
	if err != nil {
		return apierrors.FromK8sError(err, ServiceBindingResourceType)
	}
	return nil
}

func (r *ServiceBindingRepo) GetServiceBinding(ctx context.Context, authInfo authorization.Info, guid string) (ServiceBindingRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, guid, ServiceBindingResourceType)
	if err != nil {
		return ServiceBindingRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("get-service-binding failed to create user client: %w", err)
	}

	serviceBinding := &korifiv1alpha1.CFServiceBinding{}
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: guid}, serviceBinding)
	if err != nil {
		return ServiceBindingRecord{}, apierrors.FromK8sError(err, ServiceBindingResourceType)
	}

	return serviceBindingToRecord(serviceBinding)
}

func (r *ServiceBindingRepo) GetServiceBindingDetails(ctx context.Context, authInfo authorization.Info, bindingGUID string) (ServiceBindingDetailsRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	namespace, err := r.namespaceRetriever.NamespaceFor(ctx, bindingGUID, ServiceBindingResourceType)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("failed to retrieve namespace: %w", err)
	}

	serviceBinding := &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingGUID,
			Namespace: namespace,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(serviceBinding), serviceBinding)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("failed to get service binding: %w", apierrors.FromK8sError(err, ServiceBindingResourceType)) //
	}

	credentialsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      serviceBinding.Status.Credentials.Name,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("failed to get credentials: %w", apierrors.ForbiddenAsNotFound(apierrors.FromK8sError(err, ServiceBindingResourceType))) //
	}

	credentials := map[string]any{}
	err = json.Unmarshal(credentialsSecret.Data[korifiv1alpha1.CredentialsSecretKey], &credentials)
	if err != nil {
		return ServiceBindingDetailsRecord{}, fmt.Errorf("failed to unmarshal service binding credentials: %w", err)
	}

	return ServiceBindingDetailsRecord{Credentials: credentials}, nil
}

func (r *ServiceBindingRepo) UpdateServiceBinding(ctx context.Context, authInfo authorization.Info, updateMsg UpdateServiceBindingMessage) (ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to create user client: %w", err)
	}

	ns, err := r.namespaceRetriever.NamespaceFor(ctx, updateMsg.GUID, ServiceBindingResourceType)
	if err != nil {
		return ServiceBindingRecord{}, err
	}

	serviceBinding := &korifiv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      updateMsg.GUID,
			Namespace: ns,
		},
	}

	err = userClient.Get(ctx, client.ObjectKeyFromObject(serviceBinding), serviceBinding)
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to get service binding: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, serviceBinding, func() {
		updateMsg.MetadataPatch.Apply(serviceBinding)
	})
	if err != nil {
		return ServiceBindingRecord{}, fmt.Errorf("failed to patch service binding metadata: %w", apierrors.FromK8sError(err, ServiceBindingResourceType))
	}

	return serviceBindingToRecord(serviceBinding)
}

func (r *ServiceBindingRepo) GetState(ctx context.Context, authInfo authorization.Info, guid string) (model.CFResourceState, error) {
	bindingRecord, err := r.GetServiceBinding(ctx, authInfo, guid)
	if err != nil {
		return model.CFResourceStateUnknown, err
	}

	if bindingRecord.Ready {
		return model.CFResourceStateReady, nil
	}

	return model.CFResourceStateUnknown, nil
}

// nolint:dupl
func (r *ServiceBindingRepo) ListServiceBindings(ctx context.Context, authInfo authorization.Info, message ListServiceBindingsMessage) ([]ServiceBindingRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []ServiceBindingRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	authorizedSpaceNamespaces, err := authorizedSpaceNamespaces(ctx, authInfo, r.namespacePermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	labelSelector, err := labels.Parse(message.LabelSelector)
	if err != nil {
		return []ServiceBindingRecord{}, apierrors.NewUnprocessableEntityError(err, "invalid label selector")
	}

	var serviceBindings []korifiv1alpha1.CFServiceBinding
	for _, ns := range authorizedSpaceNamespaces.Collect() {
		serviceBindingList := new(korifiv1alpha1.CFServiceBindingList)
		err = userClient.List(ctx, serviceBindingList, client.InNamespace(ns), &client.ListOptions{LabelSelector: labelSelector})
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []ServiceBindingRecord{}, fmt.Errorf("failed to list service instances in namespace %s: %w",
				ns,
				apierrors.FromK8sError(err, ServiceBindingResourceType),
			)
		}
		serviceBindings = append(serviceBindings, serviceBindingList.Items...)
	}

	filteredServiceBindings := itx.FromSlice(serviceBindings).Filter(message.matches)
	return serviceBindingsToRecords(filteredServiceBindings)
}

func serviceBindingToRecord(binding *korifiv1alpha1.CFServiceBinding) (ServiceBindingRecord, error) {
	parameters := map[string]any{}
	if binding.Spec.Parameters != nil {
		err := json.Unmarshal(binding.Spec.Parameters.Raw, &parameters)
		if err != nil {
			return ServiceBindingRecord{}, fmt.Errorf("failed to unmarshal binding parameters: %w", err)
		}
	}

	return ServiceBindingRecord{
		GUID:                binding.Name,
		Type:                ServiceBindingTypeApp,
		Name:                binding.Spec.DisplayName,
		AppGUID:             binding.Spec.AppRef.Name,
		ServiceInstanceGUID: binding.Spec.Service.Name,
		SpaceGUID:           binding.Namespace,
		Labels:              binding.Labels,
		Annotations:         binding.Annotations,
		CreatedAt:           binding.CreationTimestamp.Time,
		UpdatedAt:           getLastUpdatedTime(binding),
		DeletedAt:           golangTime(binding.DeletionTimestamp),
		LastOperation:       serviceBindingRecordLastOperation(*binding),
		Ready:               isBindingReady(*binding),
		Parameters:          parameters,
	}, nil
}

func isBindingReady(binding korifiv1alpha1.CFServiceBinding) bool {
	if binding.Generation != binding.Status.ObservedGeneration {
		return false
	}

	return meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.StatusConditionReady)
}

func serviceBindingRecordLastOperation(binding korifiv1alpha1.CFServiceBinding) ServiceBindingLastOperation {
	if binding.DeletionTimestamp != nil {
		return ServiceBindingLastOperation{
			Type:      "delete",
			State:     "in progress",
			CreatedAt: binding.DeletionTimestamp.Time,
			UpdatedAt: getLastUpdatedTime(&binding),
		}
	}

	readyCondition := meta.FindStatusCondition(binding.Status.Conditions, korifiv1alpha1.StatusConditionReady)
	if readyCondition == nil {
		return ServiceBindingLastOperation{
			Type:      "create",
			State:     "initial",
			CreatedAt: binding.CreationTimestamp.Time,
			UpdatedAt: getLastUpdatedTime(&binding),
		}
	}

	if readyCondition.Status == metav1.ConditionTrue {
		return ServiceBindingLastOperation{
			Type:      "create",
			State:     "succeeded",
			CreatedAt: binding.CreationTimestamp.Time,
			UpdatedAt: getLastUpdatedTime(&binding),
		}
	}

	if meta.IsStatusConditionTrue(binding.Status.Conditions, korifiv1alpha1.BindingFailedCondition) {
		return ServiceBindingLastOperation{
			Type:      "create",
			State:     "failed",
			CreatedAt: binding.CreationTimestamp.Time,
			UpdatedAt: tools.PtrTo(readyCondition.LastTransitionTime.Time),
		}
	}

	return ServiceBindingLastOperation{
		Type:      "create",
		State:     "in progress",
		CreatedAt: binding.CreationTimestamp.Time,
		UpdatedAt: tools.PtrTo(readyCondition.LastTransitionTime.Time),
	}
}

func serviceBindingsToRecords(serviceBindings itx.Iterator[korifiv1alpha1.CFServiceBinding]) ([]ServiceBindingRecord, error) {
	serviceInstanceRecords := []ServiceBindingRecord{}

	for sb := range serviceBindings {
		record, err := serviceBindingToRecord(&sb)
		if err != nil {
			return []ServiceBindingRecord{}, err
		}
		serviceInstanceRecords = append(serviceInstanceRecords, record)
	}
	return serviceInstanceRecords, nil
}

func (r *ServiceBindingRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, bindingGUID string) (*time.Time, error) {
	serviceBinding, err := r.GetServiceBinding(ctx, authInfo, bindingGUID)
	if err != nil {
		return nil, err
	}
	return serviceBinding.DeletedAt, nil
}
