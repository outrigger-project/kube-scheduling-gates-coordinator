package webhook

import (
	"context"
	ksgcv1beta1 "github.com/outrigger-project/kube-scheduling-gates-coordinator/api/v1beta1"
	"github.com/outrigger-project/kube-scheduling-gates-coordinator/internal/model"
	"github.com/outrigger-project/kube-scheduling-gates-coordinator/internal/utils"
	v1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	log2 "sigs.k8s.io/controller-runtime/pkg/log"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//+kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,sideEffects=None,admissionReviewVersions=v1,failurePolicy=Fail,groups="",resources=pods,verbs=create;update,versions=v1,name=pod-mutating-webhook.kube-scheduling-gates-coordinator.outrigger.sh,reinvocationPolicy=IfNeeded
// +kubebuilder:rbac:groups=ksgc.outrigger.sh,resources=schedulinggatesorderings,verbs=get;list;watch;create;update;patch;delete

// PodMutator annotates Pods
type PodMutator struct {
	client     client.Client
	clientSet  *kubernetes.Clientset
	decoder    admission.Decoder
	once       sync.Once
	scheme     *runtime.Scheme
	recorder   record.EventRecorder
	workerPool *ants.MultiPool
}

func (a *PodMutator) patchedPodResponse(pod *corev1.Pod, req admission.Request) admission.Response {
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

func (a *PodMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	a.once.Do(func() {
		a.decoder = admission.NewDecoder(a.scheme)
	})
	pod := &model.Pod{}
	err := a.decoder.Decode(req, &pod.Pod)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	log := log2.FromContext(ctx).WithValues("namespace", pod.Namespace, "name", pod.Name)

	var sgo ksgcv1beta1.SchedulingGatesOrdering
	// get the sgo object from the cache
	err = a.client.Get(ctx, client.ObjectKey{
		Name: utils.SchedulingGatesOrderingName,
	}, &sgo)
	if err != nil {
		if errors.IsNotFound(err) {
			log.V(3).Info("SchedulingGatesOrdering object not found")
			return a.patchedPodResponse(&pod.Pod, req)
		}
		log.Error(err, "Failed to get SchedulingGatesOrdering object")
		return admission.Errored(http.StatusInternalServerError, err)
	}
	switch req.Operation {
	case v1.Create:
		// Ensure order of scheduling gates when they are added to the pod
		pod.EnsureLabel(utils.LabelSchedulingGateOrdering, utils.SchedulingGateOrderingStateInitial)
		pod.EnsureAndIncrementLabel(utils.LabelSchedulingGateOrderingInvocations)
		pod.EnsureSortedSchedulingGates(&sgo)
		pod.EnsureLabel(utils.LabelSchedulingGateOrdering, utils.SchedulingGateOrderingStateProcessed)
		log.V(3).Info("Scheduling gate added to the pod, launching the event creation goroutine")
		a.delayedSchedulingGatedEvent(ctx, pod.DeepCopy())
	case v1.Update:
		oldPod := &model.Pod{}
		err = a.decoder.DecodeRaw(req.OldObject, &oldPod.Pod)
		if err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		// Ensure pod modifications happen in the right order: only scheduling gates at the beginning of the list can be removed
		if pod.OutOfOrderSchedulingGatesRemoval(&oldPod.Pod) {
			log.V(3).Info("Pod has out of order scheduling gates removal, rejecting the request")
			return admission.Denied("Pod has out of order scheduling gates removal")
		}
	}
	return a.patchedPodResponse(&pod.Pod, req)
}

func (a *PodMutator) delayedSchedulingGatedEvent(ctx context.Context, pod *corev1.Pod) {
	err := a.workerPool.Submit(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		log := log2.FromContext(ctx).WithValues("namespace", pod.Namespace, "name", pod.Name,
			"function", "delayedSchedulingGatedEvent")
		// We try to get the pod from the API with exponential backoff until we find it or a timeout is reached
		err := wait.ExponentialBackoff(wait.Backoff{
			// The maximum time, excluding the time for the execution of the request,
			// is the sum of a geometric series with factor != 1.
			// maxTime = duration * (factor^steps - 1) / (factor - 1)
			// maxTime = 2e-3s * (2^15 - 1) = 65.534s
			Duration: 2 * time.Millisecond,
			Factor:   2,
			Steps:    15,
		}, func() (bool, error) {
			createdPod, err := a.clientSet.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
			if err == nil {
				log.V(2).Info("Pod was found", "namespace", pod.Namespace, "name", pod.Name)
				a.recorder.Event(createdPod, corev1.EventTypeNormal, utils.SchedulingGateOrdering, utils.SchedulingGateOrderingMessage)
				// Pod was found, return true to stop retrying
				return true, nil
			}
			if errors.IsNotFound(err) {
				log.V(3).Info("Pod not found yet", "namespace", pod.Namespace, "name", pod.Name)
				// Pod not found yet, continue retrying
				return false, nil
			}
			// Stop retrying
			log.V(3).Info("Failed to get pod", "error", err)
			return false, err
		})
		if err != nil {
			log.V(2).Info("Failed to get a scheduling gated Pod after retries",
				"error", err)
		}
	})
	if err != nil {
		log2.FromContext(ctx).WithValues("namespace", pod.Namespace, "name", pod.Name,
			"function", "delayedSchedulingGatedEvent").Error(err, "Failed to submit the delayedSchedulingGatedEvent job")
	}
}

func NewPodMutator(client client.Client, clientSet *kubernetes.Clientset,
	scheme *runtime.Scheme, recorder record.EventRecorder, workerPool *ants.MultiPool) *PodMutator {
	a := &PodMutator{
		client:     client,
		clientSet:  clientSet,
		scheme:     scheme,
		recorder:   recorder,
		workerPool: workerPool,
	}
	// TODO: metrics.InitWebhookMetrics()
	return a
}
