package guestbook

import (
	"context"
	"fmt"

	gurunathv1alpha1 "github.com/gurunath/guestbook/pkg/apis/gurunath/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_guestbook")

// ReconcileGuestbook reconciles a Guestbook object
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=deployments,verbs=get;watch;list
type ReconcileGuestbook struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new Guestbook Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGuestbook{
		client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor("guestbook-controller"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("guestbook-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource Guestbook
	err = c.Watch(&source.Kind{Type: &gurunathv1alpha1.Guestbook{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &gurunathv1alpha1.Guestbook{},
	})

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &gurunathv1alpha1.Guestbook{},
	})

	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileGuestbook implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileGuestbook{}

func (r *ReconcileGuestbook) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Guestbook")

	// Fetch the Guestbook instance
	guestbook := &gurunathv1alpha1.Guestbook{}
	err := r.client.Get(context.TODO(), request.NamespacedName, guestbook)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists for master, if not create a new one
	master := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "redis-master", Namespace: guestbook.Namespace}, master)
	if err != nil && errors.IsNotFound(err) {
		dep := r.deploymentForMaster(guestbook)
		reqLogger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}

		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Created", fmt.Sprintf("Created redis master deployment %s/%s", dep.Namespace, dep.Name))
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Deployment")
		return reconcile.Result{}, err
	}

	// Ensure the master deployment size is 1
	var masterSize int32 = 1
	if *master.Spec.Replicas != masterSize {
		master.Spec.Replicas = &masterSize
		err = r.client.Update(context.TODO(), master)
		if err != nil {
			reqLogger.Error(err, "Failed to update Deployment", "Deployment.Namespace", master.Namespace, "Deployment.Name", master.Name)
			return reconcile.Result{}, err
		}
		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Scaled", fmt.Sprintf("Scaled redis master deployment %q to %d replicas", *master.Spec.Replicas, masterSize))
		return reconcile.Result{Requeue: true}, nil
	}

	masterPodList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(master.Namespace),
		client.MatchingLabels(getLabels(guestbook.Name, "redis-master")),
	}

	if err = r.client.List(context.TODO(), masterPodList, listOpts...); err != nil {
		reqLogger.Error(err, "Failed to list pods", "master.Namespace", master.Namespace, "master.Name", master.Name)
		return reconcile.Result{}, err
	}

	masterService := corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "redis-master", Namespace: guestbook.Namespace}, &masterService)
	if err != nil && errors.IsNotFound(err) {
		ser := r.createService(guestbook, "redis-master")
		reqLogger.Info("Creating a new Service", "Service.Namespace", ser.Namespace, "Service.Name", ser.Name)
		err = r.client.Create(context.TODO(), ser)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Service", "Service.Namespace", ser.Namespace, "Service.Name", ser.Name)
			return reconcile.Result{}, err
		}
		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Created", fmt.Sprintf("created redis master service %s for deployment %s ", ser.Name, master.Name))
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Service")
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists for slave, if not create a new one
	slave := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "redis-slave", Namespace: guestbook.Namespace}, slave)
	if err != nil && errors.IsNotFound(err) {
		dep := r.deploymentForSlave(guestbook)
		reqLogger.Info("Creating a new Slave Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Slave Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}
		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Created", fmt.Sprintf("Created redis slave deployment %s/%s", dep.Namespace, dep.Name))
		// Deployment created successfully - return and requeue
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get slave Deployment")
		return reconcile.Result{}, err
	}

	var slaveSize int32 = 2
	if *slave.Spec.Replicas != slaveSize {
		slave.Spec.Replicas = &slaveSize
		err = r.client.Update(context.TODO(), slave)
		if err != nil {
			reqLogger.Error(err, "Failed to update slave Deployment", "Deployment.Namespace", slave.Namespace, "Deployment.Name", slave.Name)
			return reconcile.Result{}, err
		}

		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Scaled", fmt.Sprintf("Scaled redis slave deployment %q to %d replicas", *slave.Spec.Replicas, slaveSize))
		return reconcile.Result{Requeue: true}, nil
	}

	slavePodList := &corev1.PodList{}
	listOptsSlave := []client.ListOption{
		client.InNamespace(master.Namespace),
		client.MatchingLabels(getLabels(guestbook.Name, "redis-slave")),
	}

	if err = r.client.List(context.TODO(), slavePodList, listOptsSlave...); err != nil {
		reqLogger.Error(err, "Failed to list pods", "slave.Namespace", slave.Namespace, "slave.Name", slave.Name)
		return reconcile.Result{}, err
	}

	slaveService := corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "redis-slave", Namespace: guestbook.Namespace}, &slaveService)
	if err != nil && errors.IsNotFound(err) {
		ser := r.createService(guestbook, "redis-slave")
		reqLogger.Info("Creating a new Slave Service", "Service.Namespace", ser.Namespace, "Service.Name", ser.Name)
		err = r.client.Create(context.TODO(), ser)
		if err != nil {
			reqLogger.Error(err, "Failed to create new slave Service", "Service.Namespace", ser.Namespace, "Service.Name", ser.Name)
			return reconcile.Result{}, err
		}
		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Created", fmt.Sprintf("created redis slave service %s for deployment %s ", ser.Name, slave.Name))
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Service")
		return reconcile.Result{}, err
	}

	// ============================================================================================
	// Check if the deployment already exists for guestbook, if not create a new one
	frontend := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "frontend", Namespace: guestbook.Namespace}, frontend)
	if err != nil && errors.IsNotFound(err) {
		dep := r.deploymentForFrontend(guestbook)
		reqLogger.Info("Creating a new frontend Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new frontend Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}
		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Created", fmt.Sprintf("Created frontend deployment %s/%s", dep.Namespace, dep.Name))
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get slave Deployment")
		return reconcile.Result{}, err
	}

	var frontendSize int32 = 3
	if *frontend.Spec.Replicas != frontendSize {
		frontend.Spec.Replicas = &frontendSize
		err = r.client.Update(context.TODO(), frontend)
		if err != nil {
			reqLogger.Error(err, "Failed to update frontend Deployment", "frontend.Namespace", frontend.Namespace, "frontend.Name", frontend.Name)
			return reconcile.Result{}, err
		}
		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Scaled", fmt.Sprintf("Scaled frontend deployment %q to %d replicas", *frontend.Spec.Replicas, frontendSize))
		return reconcile.Result{Requeue: true}, nil
	}

	frontendPodList := &corev1.PodList{}
	listOptsfrontend := []client.ListOption{
		client.InNamespace(master.Namespace),
		client.MatchingLabels(getLabels(guestbook.Name, "frontend")),
	}

	if err = r.client.List(context.TODO(), frontendPodList, listOptsfrontend...); err != nil {
		reqLogger.Error(err, "Failed to list pods", "frontend.Namespace", frontend.Namespace, "frontend.Name", frontend.Name)
		return reconcile.Result{}, err
	}

	frontendService := corev1.Service{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: "frontend", Namespace: guestbook.Namespace}, &frontendService)
	if err != nil && errors.IsNotFound(err) {
		ser := r.createServiceFrontend(guestbook, "frontend")
		reqLogger.Info("Creating a new frontend Service", "Service.Namespace", ser.Namespace, "Service.Name", ser.Name)
		err = r.client.Create(context.TODO(), ser)
		if err != nil {
			reqLogger.Error(err, "Failed to create new frontend Service", "Service.Namespace", ser.Namespace, "Service.Name", ser.Name)
			return reconcile.Result{}, err
		}
		r.recorder.Event(guestbook, corev1.EventTypeNormal, "Created", fmt.Sprintf("created frontend service %s for deployment %s ", ser.Name, slave.Name))
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Service")
		return reconcile.Result{}, err
	}

	// ============================================================================================

	guestbook.Status.RedisMasterPods = getPodNames(masterPodList.Items)
	guestbook.Status.RedisSlavePods = getPodNames(slavePodList.Items)
	guestbook.Status.GuestBookPods = getPodNames(frontendPodList.Items)

	err = r.client.Status().Update(context.TODO(), guestbook)
	if err != nil {
		reqLogger.Error(err, "Failed to update Master data")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileGuestbook) deploymentForFrontend(m *gurunathv1alpha1.Guestbook) *appsv1.Deployment {
	ls := getLabels(m.Name, "frontend")
	// image := m.Spec.RedisMaster
	var replicas int32 = 3
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "frontend",
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: m.Spec.GuestBook,
						Name:  "php-redis",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 80,
							Name:          "redis-default",
						}},
						Env: []corev1.EnvVar{{
							Name:  "GET_HOSTS_FROM",
							Value: "dns",
						}},
					}},
				},
			},
		},
	}
	// Set Memcached instance as the owner and controller
	controllerutil.SetControllerReference(m, dep, r.scheme)
	return dep
}

func (r *ReconcileGuestbook) deploymentForMaster(m *gurunathv1alpha1.Guestbook) *appsv1.Deployment {
	ls := getLabels(m.Name, "redis-master")
	// image := m.Spec.RedisMaster
	var replicas int32 = 1
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-master",
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: m.Spec.RedisMaster,
						Name:  "redis-master",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 6379,
							Name:          "redis-default",
						}},
					}},
				},
			},
		},
	}
	// Set Memcached instance as the owner and controller
	controllerutil.SetControllerReference(m, dep, r.scheme)
	return dep
}

func (r *ReconcileGuestbook) deploymentForSlave(m *gurunathv1alpha1.Guestbook) *appsv1.Deployment {
	ls := getLabels(m.Name, "redis-slave")
	// image := m.Spec.RedisMaster
	var replicas int32 = 2
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-slave",
			Namespace: m.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: m.Spec.RedisSlave,
						Name:  "redis-slave",
						Ports: []corev1.ContainerPort{{
							ContainerPort: 6379,
							Name:          "redis-default",
						}},
						Env: []corev1.EnvVar{{
							Name:  "GET_HOSTS_FROM",
							Value: "dns",
						}},
					}},
				},
			},
		},
	}
	// Set Memcached instance as the owner and controller
	controllerutil.SetControllerReference(m, dep, r.scheme)
	return dep
}

func (r *ReconcileGuestbook) createServiceFrontend(m *gurunathv1alpha1.Guestbook, name string) *corev1.Service {
	ls := getLabels(m.Name, name)
	ser := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: corev1.ServiceSpec{
			Type: "NodePort",
			Ports: []corev1.ServicePort{{
				Port: 80,
			}},
			Selector: ls,
		},
	}
	controllerutil.SetControllerReference(m, ser, r.scheme)
	return ser
}

func (r *ReconcileGuestbook) createService(m *gurunathv1alpha1.Guestbook, name string) *corev1.Service {
	ls := getLabels(m.Name, name)
	ser := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: m.Namespace,
			Labels:    ls,
		},
		Spec: corev1.ServiceSpec{

			Ports: []corev1.ServicePort{{
				Port: 6379,
			}},
			Selector: ls,
		},
	}
	controllerutil.SetControllerReference(m, ser, r.scheme)
	return ser
}

// labelsForMemcached returns the labels for selecting the resources
// belonging to the given memcached CR name.
func getLabels(name string, role string) map[string]string {
	return map[string]string{"app": "guestbook", "tier": name, "role": role}
}

func getMasterPod(pods []corev1.Pod) string {
	if len(pods) == 0 {
		return ""
	}
	return pods[0].Name

}

func getPodNames(pods []corev1.Pod) []string {
	log.WithValues("*********in get pod names", len(pods))

	var podNames []string

	for _, pod := range pods {
		log.WithValues("*********in get pod names", pod.Name)
		podNames = append(podNames, pod.Name)
	}
	return podNames
	// names := []string{"Japan", "Australia", "Germany"}
	// return names
}
