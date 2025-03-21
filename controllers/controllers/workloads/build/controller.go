package build

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//counterfeiter:generate -o fake -fake-name BuildCleaner . BuildCleaner

type BuildCleaner interface {
	Clean(ctx context.Context, app types.NamespacedName) error
}

//counterfeiter:generate -o fake -fake-name DelegateReconciler . DelegateReconciler

type DelegateReconciler interface {
	ReconcileBuild(context.Context, *korifiv1alpha1.CFBuild, *korifiv1alpha1.CFApp, *korifiv1alpha1.CFPackage) (ctrl.Result, error)
	SetupWithManager(ctrl.Manager) *builder.Builder
}

type Reconciler struct {
	log          logr.Logger
	k8sClient    client.Client
	scheme       *runtime.Scheme
	buildCleaner BuildCleaner
	delegate     DelegateReconciler
}

var packageTypeToLifecycleType = map[korifiv1alpha1.PackageType]korifiv1alpha1.LifecycleType{
	"bits":   "buildpack",
	"docker": "docker",
}

func NewReconciler(
	log logr.Logger,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	buildCleaner BuildCleaner,
	delegate DelegateReconciler,
) *Reconciler {
	return &Reconciler{
		log:          log,
		k8sClient:    k8sClient,
		scheme:       scheme,
		buildCleaner: buildCleaner,
		delegate:     delegate,
	}
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return r.delegate.SetupWithManager(mgr)
}

func (r *Reconciler) ReconcileResource(ctx context.Context, cfBuild *korifiv1alpha1.CFBuild) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	if !cfBuild.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	cfBuild.Status.ObservedGeneration = cfBuild.Generation
	log.V(1).Info("set observed generation", "generation", cfBuild.Status.ObservedGeneration)

	cfApp := new(korifiv1alpha1.CFApp)
	err := r.k8sClient.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.AppRef.Name, Namespace: cfBuild.Namespace}, cfApp)
	if err != nil {
		log.Info("error when fetching CFApp", "reason", err)
		return ctrl.Result{}, err
	}

	err = r.buildCleaner.Clean(ctx, types.NamespacedName{Name: cfApp.Name, Namespace: cfBuild.Namespace})
	if err != nil {
		log.Info("unable to clean up old builds", "reason", err)
	}

	succeededStatus := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)
	if succeededStatus != nil {
		log.Info("build status indicates completion", "status", succeededStatus)
		return ctrl.Result{}, nil
	}

	err = controllerutil.SetControllerReference(cfApp, cfBuild, r.scheme)
	if err != nil {
		log.Info("unable to set owner reference on CFBuild", "reason", err)
		return ctrl.Result{}, err
	}

	cfPackage := new(korifiv1alpha1.CFPackage)
	err = r.k8sClient.Get(ctx, types.NamespacedName{Name: cfBuild.Spec.PackageRef.Name, Namespace: cfBuild.Namespace}, cfPackage)
	if err != nil {
		log.Info("error when fetching CFPackage", "reason", err)
		return ctrl.Result{}, err
	}

	err = validateLifecycleTypes(cfApp, cfPackage, cfBuild)
	if err != nil {
		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.SucceededConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "BuildFailed",
			Message:            err.Error(),
			ObservedGeneration: cfBuild.Generation,
		})

		meta.SetStatusCondition(&cfBuild.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.StagingConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "BuildNotRunning",
			ObservedGeneration: cfBuild.Generation,
		})

		return ctrl.Result{}, nil
	}

	return r.delegate.ReconcileBuild(ctx, cfBuild, cfApp, cfPackage)
}

func validateLifecycleTypes(
	cfApp *korifiv1alpha1.CFApp,
	cfPackage *korifiv1alpha1.CFPackage,
	cfBuild *korifiv1alpha1.CFBuild,
) error {
	if cfBuild.Spec.Lifecycle.Type != packageTypeToLifecycleType[cfPackage.Spec.Type] {
		return fmt.Errorf(
			"cannot build %s package with %s build",
			cfPackage.Spec.Type,
			cfBuild.Spec.Lifecycle.Type,
		)
	}

	if cfApp.Spec.Lifecycle.Type != packageTypeToLifecycleType[cfPackage.Spec.Type] {
		return fmt.Errorf(
			"cannot build %s package for %s app",
			cfPackage.Spec.Type,
			cfApp.Spec.Lifecycle.Type,
		)
	}

	return nil
}
