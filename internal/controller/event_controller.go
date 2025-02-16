/*
Copyright 2025.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	authv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "mallory-operator/api/v1"
)

const (
	finalizerName      = "events.mallory.io/finalizer"
	EventTypeWarning   = corev1.EventTypeWarning
	EventTypeNormal    = corev1.EventTypeNormal
	OperationFailed    = "OperationFailed"
	OperationSucceeded = "OperationSucceeded"
	OperationOutput    = "OperationOutput"
)

// EventReconciler reconciles a Event object
type EventReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *rest.Config
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=mallory.io,resources=events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mallory.io,resources=events/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mallory.io,resources=events/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=users;groups;serviceaccounts,verbs=impersonate

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Event object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
// Constants for the finalizer name and event types

// Reconcile is part of the main Kubernetes reconciliation loop.
func (r *EventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	var event v1.Event
	if err := r.Get(ctx, req.NamespacedName, &event); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.manageFinalizer(ctx, &event); err != nil {
		return ctrl.Result{}, err
	}

	result := r.processOperations(ctx, &event, req.Namespace)
	if err := r.updateEventStatus(ctx, &event, result); err != nil {
		return ctrl.Result{}, err
	}

	if !event.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.cleanupResources(ctx, &event, req.Namespace); err != nil {
			logger.Error(err, "failed to clean up resources")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *EventReconciler) manageFinalizer(ctx context.Context, event *v1.Event) error {
	if event.ObjectMeta.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(event, finalizerName) {
			controllerutil.AddFinalizer(event, finalizerName)
			return r.Update(ctx, event)
		}
	}
	return nil
}

func (r *EventReconciler) processOperations(ctx context.Context, event *v1.Event, namespace string) string {
	result := "Success"
	for _, operation := range event.Spec.Operations {
		if err := r.processSingleOperation(ctx, event, namespace, operation); err != nil {
			result = "Error"
		}
	}
	return result
}

func (r *EventReconciler) processSingleOperation(ctx context.Context, event *v1.Event, namespace string, operation *v1.Operation) error {
	output, err := r.processResourceOperation(ctx, event.Spec.Intruder, namespace, operation)
	if err != nil {
		r.Recorder.Event(event, EventTypeWarning, OperationFailed, fmt.Sprintf("failed to process resource operation: %v", err))
		return err
	}
	r.Recorder.Event(event, EventTypeNormal, OperationSucceeded, "Resource processed successfully")
	if output != "" {
		r.Recorder.Event(event, EventTypeNormal, OperationOutput, output)
	}
	return nil
}

func (r *EventReconciler) updateEventStatus(ctx context.Context, event *v1.Event, result string) error {
	event.Status.Result = result
	return r.Status().Update(ctx, event)
}

func (r *EventReconciler) cleanupResources(ctx context.Context, event *v1.Event, namespace string) error {
	for _, operation := range event.Spec.Operations {
		if operation.Verb == "create" {
			if err := r.deleteResource(ctx, event, namespace, operation); err != nil {
				return err
			}
		}
	}
	controllerutil.RemoveFinalizer(event, finalizerName)
	return r.Update(ctx, event)
}

func (r *EventReconciler) deleteResource(ctx context.Context, event *v1.Event, namespace string, operation *v1.Operation) error {
	var obj unstructured.Unstructured
	if err := json.Unmarshal(operation.Resource.Raw, &obj.Object); err != nil {
		return fmt.Errorf("failed to unmarshal resource: %v", err)
	}

	ns := obj.GetNamespace()
	if ns == "" {
		ns = namespace
		obj.SetNamespace(namespace)
	}

	if err := r.deleteResourceWithIntruder(ctx, event.Spec.Intruder, ns, &obj); err != nil {
		return fmt.Errorf("failed to delete resource %s: %v", operation.ID, err)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Event{}).
		Complete(r)
}

// buildIntruderClient creates a Kubernetes client using either a token or impersonation.
func (r *EventReconciler) buildIntruderClient(intruder v1.Intruder, namespace string) (client.Client, error) {
	return client.New(r.generateIntruderRestConfig(intruder, namespace), client.Options{Scheme: r.Scheme})
}

func (r *EventReconciler) generateIntruderRestConfig(intruder v1.Intruder, namespace string) *rest.Config {
	var restConfig *rest.Config

	if intruder.Token != "" {
		restConfig = &rest.Config{
			Host:            r.Config.Host,
			TLSClientConfig: r.Config.TLSClientConfig,
			BearerToken:     intruder.Token,
		}
	} else if intruder.ServiceAccount != "" {
		restConfig = rest.CopyConfig(r.Config)
		restConfig.Impersonate = rest.ImpersonationConfig{
			UserName: fmt.Sprintf("system:serviceaccount:%s:%s", namespace, intruder.ServiceAccount),
		}
	} else if intruder.User != nil {
		restConfig = rest.CopyConfig(r.Config)
		restConfig.Impersonate = rest.ImpersonationConfig{
			UserName: intruder.User.Name,
			Groups:   intruder.User.Groups,
		}
	} else {
		restConfig = r.Config
	}
	return restConfig
}

// deleteResourceWithIntruder удаляет ресурс с использованием клиента, настроенного через Intruder.
func (r *EventReconciler) deleteResourceWithIntruder(ctx context.Context, intruder v1.Intruder, namespace string, obj *unstructured.Unstructured) error {
	cl, err := r.buildIntruderClient(intruder, namespace)
	if err != nil {
		return fmt.Errorf("failed to create intruder client: %w", err)
	}
	if err := cl.Delete(ctx, obj); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

// processResourceOperation performs an operation (create, delete, get, etc.) on a Kubernetes resource.
func (r *EventReconciler) processResourceOperation(ctx context.Context, intruder v1.Intruder, namespace string, op *v1.Operation) (string, error) {

	cl, err := r.buildIntruderClient(intruder, namespace)
	if err != nil {
		return "", fmt.Errorf("failed to create intruder client: %w", err)
	}

	var obj unstructured.Unstructured
	if err := json.Unmarshal(op.Resource.Raw, &obj.Object); err != nil {
		return "", fmt.Errorf("failed to unmarshal resource template: %w", err)
	}

	if obj.GetNamespace() == "" {
		obj.SetNamespace(namespace)
	}
	restConfig := r.generateIntruderRestConfig(intruder, namespace)
	fmt.Println(namespace, intruder)
	switch op.Verb {
	case "create":
		return r.handleCreate(ctx, cl, obj)
	case "delete":
		return "", cl.Delete(ctx, &obj)
	case "get":
		return r.handleGet(ctx, cl, obj)
	case "list":
		return r.handleList(ctx, cl, obj)
	case "update":
		return "", cl.Update(ctx, &obj)
	case "exec":
		return r.handleExec(ctx, cl, restConfig, obj)
	case "auth":
		return r.handleAuth(ctx, cl, op)
	case "logs":
		return r.handleLogs(ctx, cl, restConfig, obj)
	default:
		return "", fmt.Errorf("unsupported verb: %s", op.Verb)
	}
}

func (r *EventReconciler) handleLogs(ctx context.Context, cl client.Client, rc *rest.Config, obj unstructured.Unstructured) (string, error) {
	listObj := &unstructured.UnstructuredList{}
	listObj.SetAPIVersion(obj.GetAPIVersion())
	listObj.SetKind(obj.GetKind() + "List")
	if err := cl.List(ctx, listObj, client.InNamespace(obj.GetNamespace()), client.MatchingLabels(obj.GetLabels())); err != nil {
		if errors.IsNotFound(err) {
			return err.Error(), nil
		}
		return "", err
	}
	for _, item := range listObj.Items {
		// TODO: support multiple answers
		return r.GetLogs(item, rc)
	}
	return "", nil
}

// handleCreate performs an idempotent create operation.
func (r *EventReconciler) handleCreate(ctx context.Context, cl client.Client, obj unstructured.Unstructured) (string, error) {
	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(obj.GetAPIVersion())
	existing.SetKind(obj.GetKind())
	key := client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}

	if err := cl.Get(ctx, key, existing); err == nil {
		return "", nil // Object already exists
	} else if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to get resource: %w", err)
	}

	return "", cl.Create(ctx, &obj)
}

// handleGet retrieves a resource and returns it as a JSON string.
func (r *EventReconciler) handleGet(ctx context.Context, cl client.Client, obj unstructured.Unstructured) (string, error) {
	if err := cl.Get(ctx, client.ObjectKey{Name: obj.GetName(), Namespace: obj.GetNamespace()}, &obj); err != nil {
		if errors.IsNotFound(err) {
			return err.Error(), nil
		}
		return "", err
	}
	b, err := json.Marshal(obj.Object)
	return string(b), err
}

// handleList retrieves a list of resources matching the labels of the given object.
func (r *EventReconciler) handleList(ctx context.Context, cl client.Client, obj unstructured.Unstructured) (string, error) {
	listObj := &unstructured.UnstructuredList{}
	listObj.SetAPIVersion(obj.GetAPIVersion())
	listObj.SetKind(obj.GetKind() + "List")

	if err := cl.List(ctx, listObj, client.InNamespace(obj.GetNamespace()), client.MatchingLabels(obj.GetLabels())); err != nil {
		if errors.IsNotFound(err) {
			return err.Error(), nil
		}
		return "", err
	}

	var names []string
	for _, item := range listObj.Items {
		names = append(names, item.GetName())
	}

	b, err := json.Marshal(names)
	return string(b), err
}

// handleAuth performs a SelfSubjectAccessReview authorization check.
func (r *EventReconciler) handleAuth(ctx context.Context, cl client.Client, op *v1.Operation) (string, error) {
	var ssar authv1.SelfSubjectAccessReview
	if err := json.Unmarshal(op.Resource.Raw, &ssar); err != nil {
		return "", fmt.Errorf("error unmarshaling to SelfSubjectAccessReview: %w", err)
	}
	if err := cl.Create(ctx, &ssar); err != nil {
		return "", err
	}
	return ssar.Status.String(), nil
}

// handleExec executes a command inside a pod.
func (r *EventReconciler) handleExec(ctx context.Context, cl client.Client, rc *rest.Config, obj unstructured.Unstructured) (string, error) {
	listObj := &unstructured.UnstructuredList{}
	listObj.SetAPIVersion(obj.GetAPIVersion())
	listObj.SetKind(obj.GetKind() + "List")
	if err := cl.List(ctx, listObj, client.InNamespace(obj.GetNamespace()), client.MatchingLabels(obj.GetLabels())); err != nil {
		if errors.IsNotFound(err) {
			return err.Error(), nil
		}
		return "", err
	}

	for _, item := range listObj.Items {
		name, cmd, err := extractExecDetails(obj)
		if err != nil {
			return "", err
		}

		// Создаём clientset из client-go для выполнения операции exec
		clientset, err := kubernetes.NewForConfig(rc)
		if err != nil {
			return "", err
		}
		// Формируем запрос на exec
		req := clientset.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(item.GetName()).
			Namespace(item.GetNamespace()).
			SubResource("exec").
			VersionedParams(&corev1.PodExecOptions{
				Container: name,
				Command:   cmd,
				Stdin:     false,
				Stdout:    true,
				Stderr:    true,
				TTY:       false,
			}, runtime.NewParameterCodec(scheme.Scheme))

		// Создаём исполнитель (executor) для команды exec
		exec, err := remotecommand.NewSPDYExecutor(rc, "POST", req.URL())
		if err != nil {
			return "", fmt.Errorf("Ошибка создания SPDYExecutor: %v\n", err)
		}

		// Потоки для сбора вывода команды
		var stdout, stderr bytes.Buffer

		// Выполняем команду в контейнере
		err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
			Stdin:  nil,
			Stdout: &stdout,
			Stderr: &stderr,
			Tty:    false,
		})
		if err != nil {
			return "", fmt.Errorf("Ошибка выполнения команды exec: %v, stderr: %s", err, stderr.String())
		}

		// TODO: change to multiple output
		return stdout.String(), nil
	}
	return "", nil
}

// extractExecDetails takes a runtime.RawExtension, which describes a resource
// (Pod, Deployment, ReplicaSet, etc.), and extracts the container name, command, and args.
// The main requirement is that the container description must contain the fields `name` and `command`.
func extractExecDetails(obj unstructured.Unstructured) (containerName string, command []string, err error) {
	// Determine the resource Kind
	kind, found, err := unstructured.NestedString(obj.Object, "kind")
	if err != nil || !found {
		return "", nil, fmt.Errorf("the 'kind' field was not found in the resource")
	}

	// Define the path to containers based on the resource Kind
	var containers []interface{}
	switch kind {
	case "Pod":
		containers, found, err = unstructured.NestedSlice(obj.Object, "spec", "containers")
		if err != nil || !found {
			return "", nil, fmt.Errorf("containers not found in Pod spec")
		}
	case "Deployment", "ReplicaSet":
		containers, found, err = unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
		if err != nil || !found {
			return "", nil, fmt.Errorf("containers not found in %s spec", kind)
		}
	default:
		return "", nil, fmt.Errorf("unsupported kind: %s", kind)
	}

	if len(containers) == 0 {
		return "", nil, fmt.Errorf("container list is empty")
	}

	// Select the first container (this logic can be extended if a specific container needs to be chosen)
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return "", nil, fmt.Errorf("unexpected container format")
	}

	// Extract the container name
	containerName, found, err = unstructured.NestedString(container, "name")
	if err != nil || !found {
		return "", nil, fmt.Errorf("container 'name' field not found")
	}

	// Extract the 'command' field (expected to be a string array)
	var cmdInterface interface{}
	cmdInterface, found, err = unstructured.NestedFieldNoCopy(container, "command")
	if err != nil || !found {
		return containerName, nil, fmt.Errorf("the 'command' field was not found in the container")
	}
	command, err = interfaceSliceToStringSlice(cmdInterface)
	if err != nil {
		return containerName, nil, fmt.Errorf("failed to convert 'command' to []string: %w", err)
	}

	// Extract the 'args' field if it exists (it's optional)
	var argsInterface interface{}
	argsInterface, found, err = unstructured.NestedFieldNoCopy(container, "args")
	if found && err == nil {
		args, err := interfaceSliceToStringSlice(argsInterface)
		if err != nil {
			return containerName, command, fmt.Errorf("failed to convert 'args' to []string: %w", err)
		}
		command = append(command, args...)
	}

	return containerName, command, nil
}

// interfaceSliceToStringSlice converts an interface{} to []string
// if the original object is a slice where all elements are strings.
func interfaceSliceToStringSlice(i interface{}) ([]string, error) {
	slice, ok := i.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected a slice but got %T", i)
	}
	var result []string
	for _, v := range slice {
		str, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("element is not a string: %v", v)
		}
		result = append(result, str)
	}
	return result, nil
}

// GetLogs retrieves logs from a given Kubernetes resource.
func (r *EventReconciler) GetLogs(obj unstructured.Unstructured, rc *rest.Config) (string, error) {
	// Create a discovery client and RESTMapper.
	discClient, err := discovery.NewDiscoveryClientForConfig(rc)
	if err != nil {
		return "", fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Construct the log request path.
	fullPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log", obj.GetNamespace(), obj.GetName())
	req := discClient.RESTClient().Get().AbsPath(fullPath).Param("tailLines", "10")

	stream, err := req.Stream(context.Background())
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer stream.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, stream)
	return buf.String(), err
}
