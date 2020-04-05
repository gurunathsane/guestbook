package guestbook

import (
	"context"

	gurunathv1alpha1 "github.com/gurunath/guestbook/pkg/apis/gurunath/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	return &ReconcileGuestbook{client: mgr.GetClient(), scheme: mgr.GetScheme()}
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

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner Guestbook
	err = c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
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

// ReconcileGuestbook reconciles a Guestbook object
type ReconcileGuestbook struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Guestbook object and makes changes based on the state read
// and what is in the Guestbook.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileGuestbook) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Guestbook")

	// Fetch the Guestbook instance
	guestbook := &gurunathv1alpha1.Guestbook{}
	err := r.client.Get(context.TODO(), request.NamespacedName, guestbook)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists for master, if not create a new one
	master := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: guestbook.Name + "-redis-master", Namespace: guestbook.Namespace}, master)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment for master
		dep := r.deploymentForMaster(guestbook)
		reqLogger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}
		// Deployment created successfully - return and requeue
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
		// Spec updated - return and requeue
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the Guestbook status with the pod names
	// List the pods for this master deployment
	masterPodList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(master.Namespace),
		client.MatchingLabels(getLabels(guestbook.Name, "redis-master")),
	}

	if err = r.client.List(context.TODO(), masterPodList, listOpts...); err != nil {
		reqLogger.Error(err, "Failed to list pods", "master.Namespace", master.Namespace, "master.Name", master.Name)
		return reconcile.Result{}, err
	}

	// Check if the deployment already exists for slave, if not create a new one
	slave := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: guestbook.Name + "-redis-slave", Namespace: guestbook.Namespace}, slave)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment for master
		dep := r.deploymentForSlave(guestbook)
		reqLogger.Info("Creating a new Slave Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Slave Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}
		// Deployment created successfully - return and requeue
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get slave Deployment")
		return reconcile.Result{}, err
	}

	// Ensure the slave deployment size is 1
	var slaveSize int32 = 2
	if *slave.Spec.Replicas != slaveSize {
		slave.Spec.Replicas = &masterSize
		err = r.client.Update(context.TODO(), slave)
		if err != nil {
			reqLogger.Error(err, "Failed to update slave Deployment", "Deployment.Namespace", slave.Namespace, "Deployment.Name", slave.Name)
			return reconcile.Result{}, err
		}
		// Spec updated - return and requeue
		return reconcile.Result{Requeue: true}, nil
	}

	// Update the Guestbook status with the pod names
	// List the pods for this master deployment
	slavePodList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(master.Namespace),
		client.MatchingLabels(getLabels(guestbook.Name, "redis-slave")),
	}

	if err = r.client.List(context.TODO(), masterPodList, listOpts...); err != nil {
		reqLogger.Error(err, "Failed to list slave pods", "master.Namespace", slave.Namespace, "slave.Name", slave.Name)
		return reconcile.Result{}, err
	}

	guestbook.Status.RedisMasterPods = getPodNames(masterPodList.Items)
	guestbook.Status.RedisMasterDeployment = master.Name
	guestbook.Status.RedisSlaveDeployment = "guru"
	guestbook.Status.RedisSlavePods = getPodNames(slavePodList.Items)
	guestbook.Status.GuestBookDeployment = "one"
	guestbook.Status.GuestBookPods = getPodNames(masterPodList.Items)

	err = r.client.Status().Update(context.TODO(), guestbook)
	if err != nil {
		reqLogger.Error(err, "Failed to update Master data")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileGuestbook) deploymentForMaster(m *gurunathv1alpha1.Guestbook) *appsv1.Deployment {
	ls := getLabels(m.Name, "redis-master")
	// image := m.Spec.RedisMaster
	var replicas int32 = 1
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Name + "-redis-master",
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
			Name:      m.Name + "-redis-slave",
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
	var podNames []string

	for _, pod := range pods {
		podNames = append(podNames, pod.Name)
	}
	return podNames
	// names := []string{"Japan", "Australia", "Germany"}
	// return names
}
