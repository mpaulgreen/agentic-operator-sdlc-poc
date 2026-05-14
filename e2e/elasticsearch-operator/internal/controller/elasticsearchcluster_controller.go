/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	searchv1alpha1 "github.com/example/elasticsearch-operator/api/v1alpha1"
)

const (
	elasticsearchClusterFinalizer = "search.elasticsearch.example.com/finalizer"
)

// ElasticsearchClusterReconciler reconciles a ElasticsearchCluster object.
type ElasticsearchClusterReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=search.elasticsearch.example.com,resources=elasticsearchclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=search.elasticsearch.example.com,resources=elasticsearchclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=search.elasticsearch.example.com,resources=elasticsearchclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

func (r *ElasticsearchClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// --- PHASE 1: FETCH ---
	cr := &searchv1alpha1.ElasticsearchCluster{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ElasticsearchCluster resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !cr.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, cr)
	}

	// --- PHASE 2: ORCHESTRATE ---
	if !controllerutil.ContainsFinalizer(cr, elasticsearchClusterFinalizer) {
		controllerutil.AddFinalizer(cr, elasticsearchClusterFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	if cr.Status.Phase == "" {
		cr.Status.Phase = "Initializing"
		if err := r.Status().Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileSecret(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "SecretReconcileFailed", err)
	}
	if err := r.reconcileConfigMap(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "ConfigMapReconcileFailed", err)
	}
	if err := r.reconcileHTTPService(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "HTTPServiceReconcileFailed", err)
	}
	if err := r.reconcileTransportService(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "TransportServiceReconcileFailed", err)
	}
	if err := r.reconcileStatefulSet(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "StatefulSetReconcileFailed", err)
	}
	if err := r.reconcileMaster(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "MasterReconcileFailed", err)
	}
	if err := r.reconcileBackupCronJob(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "BackupCronJobReconcileFailed", err)
	}
	if err := r.reconcileNetworkPolicy(ctx, cr); err != nil {
		return r.handleError(ctx, cr, "NetworkPolicyReconcileFailed", err)
	}

	// --- PHASE 3: STATUS ---
	if err := r.updateStatus(ctx, cr); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *ElasticsearchClusterReconciler) handleDeletion(ctx context.Context, cr *searchv1alpha1.ElasticsearchCluster) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(cr, elasticsearchClusterFinalizer) {
		logger.Info("Performing cleanup for ElasticsearchCluster", "name", cr.Name)
		r.Recorder.Event(cr, corev1.EventTypeNormal, "CleanupStarted",
			fmt.Sprintf("Cleaning up resources for ElasticsearchCluster %s", cr.Name))

		if err := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cr); err != nil {
			return ctrl.Result{}, err
		}
		controllerutil.RemoveFinalizer(cr, elasticsearchClusterFinalizer)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}

		r.Recorder.Event(cr, corev1.EventTypeNormal, "CleanupCompleted",
			fmt.Sprintf("Cleanup completed for ElasticsearchCluster %s", cr.Name))
	}

	return ctrl.Result{}, nil
}

func (r *ElasticsearchClusterReconciler) handleError(ctx context.Context, cr *searchv1alpha1.ElasticsearchCluster, reason string, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Error(err, "Reconciliation failed", "reason", reason)
	r.Recorder.Event(cr, corev1.EventTypeWarning, reason, err.Error())

	cr.Status.Phase = "Failed"
	setDegradedCondition(cr, reason, err.Error())
	if statusErr := r.Status().Update(ctx, cr); statusErr != nil {
		logger.Error(statusErr, "Failed to update status after error")
	}

	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *ElasticsearchClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&searchv1alpha1.ElasticsearchCluster{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&batchv1.CronJob{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Complete(r)
}
