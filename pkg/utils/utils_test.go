package utils

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/application-stacks/runtime-component-operator/pkg/apis/appstacks/v1beta1"
	appstacksv1beta1 "github.com/application-stacks/runtime-component-operator/pkg/apis/appstacks/v1beta1"
	"github.com/application-stacks/runtime-component-operator/pkg/common"
	prometheusv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	routev1 "github.com/openshift/api/route/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	name               = "my-app"
	namespace          = "runtime"
	stack              = "java-microprofile"
	appImage           = "my-image"
	replicas     int32 = 2
	expose             = true
	createKNS          = true
	targetCPUPer int32 = 30
	targetPort   int32 = 3333
	nodePort     int32 = 3011
	autoscaling        = &appstacksv1beta1.RuntimeComponentAutoScaling{
		TargetCPUUtilizationPercentage: &targetCPUPer,
		MinReplicas:                    &replicas,
		MaxReplicas:                    3,
	}
	envFrom                  = []corev1.EnvFromSource{{Prefix: namespace}}
	env                      = []corev1.EnvVar{{Name: namespace}}
	pullPolicy               = corev1.PullAlways
	pullSecret               = "mysecret"
	serviceAccountName       = "service-account"
	serviceType              = corev1.ServiceTypeClusterIP
	serviceType2             = corev1.ServiceTypeNodePort
	provides                 = appstacksv1beta1.ServiceBindingProvides{Context: "/path", Protocol: "TCP"}
	consumes                 = appstacksv1beta1.ServiceBindingConsumes{Name: "consumes"}
	consume                  = []appstacksv1beta1.ServiceBindingConsumes{consumes}
	service                  = &appstacksv1beta1.RuntimeComponentService{Type: &serviceType, Port: 8443, Provides: &provides, Consumes: consume}
	svcPortName              = "myservice"
	targetHelper       int32 = 9000
	ports                    = []corev1.ServicePort{corev1.ServicePort{Name: "https", Port: 9080, TargetPort: intstr.FromInt(9000)}, corev1.ServicePort{Port: targetPort}}
	volumeCT                 = &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc", Namespace: namespace},
		TypeMeta:   metav1.TypeMeta{Kind: "StatefulSet"}}
	storage        = appstacksv1beta1.RuntimeComponentStorage{Size: "10Mi", MountPath: "/mnt/data", VolumeClaimTemplate: volumeCT}
	arch           = []string{"ppc64le"}
	readinessProbe = &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet:   &corev1.HTTPGetAction{},
			TCPSocket: &corev1.TCPSocketAction{},
		},
	}
	livenessProbe = &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet:   &corev1.HTTPGetAction{},
			TCPSocket: &corev1.TCPSocketAction{},
		},
	}
	volume      = corev1.Volume{Name: "runtime-volume"}
	volumeMount = corev1.VolumeMount{Name: volumeCT.Name, MountPath: storage.MountPath}
	resLimits   = map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: {},
	}
	resourceContraints = &corev1.ResourceRequirements{Limits: resLimits}
	labels             = map[string]string{"key1": "value1"}
	annotations        = map[string]string{"key2": "value2"}
	version            = "testing"
	key                = "key"
	crt                = "crt"
	ca                 = "ca"
	destCACert         = "destCACert"
	emptyString        = ""
)

type Test struct {
	test     string
	expected interface{}
	actual   interface{}
}

func TestCustomizeDeployment(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	spec := appstacksv1beta1.RuntimeComponentSpec{ApplicationName: appImage, Service: service, Version: version}
	deploy, runtime := appsv1.Deployment{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeDeployment(&deploy, runtime)

	//TestGetLabels
	testCR := []Test{
		{"Deployment labels", name, deploy.Labels["app.kubernetes.io/instance"]},
		{"Deployment labels", name, deploy.Labels["app.kubernetes.io/name"]},
		{"Deployment labels", "runtime-component-operator", deploy.Labels["app.kubernetes.io/managed-by"]},
		{"Deployment labels", "backend", deploy.Labels["app.kubernetes.io/component"]},
		{"Deployment labels", appImage, deploy.Labels["app.kubernetes.io/part-of"]},
		{"Deployment labels", version, deploy.Labels["app.kubernetes.io/version"]},
		{"Deployment labels", "value1", deploy.Labels["key1"]},
		{"Deployment labels", "true", deploy.Labels["service.app.stacks/bindable"]},
	}

	verifyTests(testCR, t)

	//TestGetAnnotations
	testCR = []Test{
		{"Deployment annotations", "value2", deploy.Annotations["key2"]},
	}

	verifyTests(testCR, t)
}

func TestCustomizeStatefulSet(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	spec := appstacksv1beta1.RuntimeComponentSpec{ApplicationName: appImage, Service: service, Version: version, Replicas: &replicas}
	statefulset, runtime := appsv1.StatefulSet{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeStatefulSet(&statefulset, runtime)

	//TestGetReplicas
	testCR := []Test{
		{"Statefulset replicas", replicas, *statefulset.Spec.Replicas},
		{"Statefulset service name", name + "-headless", statefulset.Spec.ServiceName},
		{"Statefulset selector", name, statefulset.Spec.Selector.MatchLabels["app.kubernetes.io/instance"]},
	}

	verifyTests(testCR, t)
}

func TestCustomizeRoute(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	route, runtime := &routev1.Route{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeRoute(route, runtime, "", "", "", "")

	//TestGetLabels
	testCR := []Test{
		{"Route labels", name, route.Labels["app.kubernetes.io/instance"]},
		{"Route target kind", "Service", route.Spec.To.Kind},
		{"Route target name", name, route.Spec.To.Name},
		{"Route target weight", int32(100), *route.Spec.To.Weight},
		{"Route service target port", intstr.FromString(strconv.Itoa(int(runtime.Spec.Service.Port)) + "-tcp"), route.Spec.Port.TargetPort},
	}

	verifyTests(testCR, t)

	helper := routev1.TLSTerminationEdge
	helper2 := routev1.InsecureEdgeTerminationPolicyNone
	runtime.Spec.Route = &v1beta1.RuntimeComponentRoute{Termination: &helper, InsecureEdgeTerminationPolicy: &helper2}

	CustomizeRoute(route, runtime, key, crt, ca, destCACert)

	//TestEdge
	testCR = []Test{
		{"Route Certificate", crt, route.Spec.TLS.Certificate},
		{"Route CACertificate", ca, route.Spec.TLS.CACertificate},
		{"Route Key", key, route.Spec.TLS.Key},
		{"Route DestinationCertificate", "", route.Spec.TLS.DestinationCACertificate},
		{"Route InsecureEdgeTerminationPolicy", helper2, route.Spec.TLS.InsecureEdgeTerminationPolicy},
	}

	verifyTests(testCR, t)

	helper = routev1.TLSTerminationReencrypt
	runtime.Spec.Route = &v1beta1.RuntimeComponentRoute{Termination: &helper, InsecureEdgeTerminationPolicy: &helper2}

	CustomizeRoute(route, runtime, key, crt, ca, destCACert)

	//TestReencrypt
	testCR = []Test{
		{"Route Certificate", crt, route.Spec.TLS.Certificate},
		{"Route CACertificate", ca, route.Spec.TLS.CACertificate},
		{"Route Key", key, route.Spec.TLS.Key},
		{"Route DestinationCertificate", destCACert, route.Spec.TLS.DestinationCACertificate},
		{"Route InsecureEdgeTerminationPolicy", helper2, route.Spec.TLS.InsecureEdgeTerminationPolicy},
		{"Route Target Port", "8443-tcp", route.Spec.Port.TargetPort.StrVal},
	}
	verifyTests(testCR, t)

	helper = routev1.TLSTerminationPassthrough
	runtime.Spec.Route = &v1beta1.RuntimeComponentRoute{Termination: &helper}
	runtime.Spec.Service.PortName = svcPortName

	CustomizeRoute(route, runtime, key, crt, ca, destCACert)

	//TestPassthrough
	testCR = []Test{
		{"Route Certificate", "", route.Spec.TLS.Certificate},
		{"Route CACertificate", "", route.Spec.TLS.CACertificate},
		{"Route Key", "", route.Spec.TLS.Key},
		{"Route DestinationCertificate", "", route.Spec.TLS.DestinationCACertificate},
		{"Route Target Port", svcPortName, route.Spec.Port.TargetPort.StrVal},
	}

	verifyTests(testCR, t)
}

func TestErrorIsNoMatchesForKind(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	newError := errors.New("test error")
	errorValue := ErrorIsNoMatchesForKind(newError, "kind", "version")

	testCR := []Test{
		{"Error", false, errorValue},
	}

	verifyTests(testCR, t)
}

func TestCustomizeService(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	svc, runtime := &corev1.Service{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeService(svc, runtime)
	testCS := []Test{
		{"Service number of exposed ports", 1, len(svc.Spec.Ports)},
		{"Service first exposed port", runtime.Spec.Service.Port, svc.Spec.Ports[0].Port},
		{"Service first exposed target port", intstr.FromInt(int(runtime.Spec.Service.Port)), svc.Spec.Ports[0].TargetPort},
		{"Service type", *runtime.Spec.Service.Type, svc.Spec.Type},
		{"Service selector", name, svc.Spec.Selector["app.kubernetes.io/instance"]},
	}
	verifyTests(testCS, t)

	// Verify behaviour of optional target port functionality
	verifyTests(optionalTargetPortFunctionalityTests(), t)

	// verify optional nodePort functionality in NodePort service
	verifyTests(optionalNodePortFunctionalityTests(), t)

	additionalPortsTests(t)
}

func additionalPortsTests(t *testing.T) {
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	svc, runtime := &corev1.Service{}, createRuntimeComponent(name, namespace, spec, labels, annotations)
	runtime.Spec.Service.Ports = ports

	CustomizeService(svc, runtime)

	testCS := []Test{
		{"Service number of exposed ports", 3, len(svc.Spec.Ports)},
		{"Second exposed port", ports[0].Port, svc.Spec.Ports[1].Port},
		{"Second exposed target port", targetHelper, svc.Spec.Ports[1].TargetPort.IntVal},
		{"Second exposed port name", ports[0].Name, svc.Spec.Ports[1].Name},
		{"Second nodeport", ports[0].NodePort, svc.Spec.Ports[1].NodePort},
		{"Third exposed port", ports[1].Port, svc.Spec.Ports[2].Port},
		{"Third exposed port name", fmt.Sprint(ports[1].Port) + "-tcp", svc.Spec.Ports[2].Name},
		{"Third nodeport", ports[1].NodePort, svc.Spec.Ports[2].NodePort},
	}

	verifyTests(testCS, t)

	runtime.Spec.Service.Ports = runtime.Spec.Service.Ports[:len(runtime.Spec.Service.Ports)-1]
	runtime.Spec.Service.Ports[0].NodePort = 3000
	runtime.Spec.Service.Type = &serviceType2
	CustomizeService(svc, runtime)

	testCS = []Test{
		{"Service number of exposed ports", 2, len(svc.Spec.Ports)},
		{"First nodeport", 3000, svc.Spec.Ports[0].NodePort},
		{"Port type", serviceType2, svc.Spec.Type},
	}

	runtime.Spec.Service.Ports = nil
	CustomizeService(svc, runtime)

	testCS = []Test{
		{"Service number of exposed ports", 1, len(svc.Spec.Ports)},
	}

	verifyTests(testCS, t)
}

func optionalTargetPortFunctionalityTests() []Test {
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	spec.Service.TargetPort = &targetPort
	svc, runtime := &corev1.Service{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeService(svc, runtime)
	testCS := []Test{
		{"Service number of exposed ports", 1, len(svc.Spec.Ports)},
		{"Service first exposed port", runtime.Spec.Service.Port, svc.Spec.Ports[0].Port},
		{"Service first exposed target port", intstr.FromInt(int(*runtime.Spec.Service.TargetPort)), svc.Spec.Ports[0].TargetPort},
		{"Service type", *runtime.Spec.Service.Type, svc.Spec.Type},
		{"Service selector", name, svc.Spec.Selector["app.kubernetes.io/instance"]},
	}
	return testCS
}

func optionalNodePortFunctionalityTests() []Test {
	serviceType := corev1.ServiceTypeNodePort
	service := &appstacksv1beta1.RuntimeComponentService{Type: &serviceType, Port: 8443, NodePort: &nodePort}
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	svc, runtime := &corev1.Service{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeService(svc, runtime)
	testCS := []Test{
		{"Service number of exposed ports", 1, len(svc.Spec.Ports)},
		{"Sercice first exposed port", runtime.Spec.Service.Port, svc.Spec.Ports[0].Port},
		{"Service first exposed target port", intstr.FromInt(int(runtime.Spec.Service.Port)), svc.Spec.Ports[0].TargetPort},
		{"Service type", *runtime.Spec.Service.Type, svc.Spec.Type},
		{"Service selector", name, svc.Spec.Selector["app.kubernetes.io/instance"]},
		{"Service nodePort port", *runtime.Spec.Service.NodePort, svc.Spec.Ports[0].NodePort},
	}
	return testCS
}

func TestCustomizeServiceBindingSecret(t *testing.T) {
	secret := corev1.Secret{}
	auth := map[string]string{"username": "admin", "password": "admin"}

	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	runtime := createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeServiceBindingSecret(&secret, auth, runtime)

	// Test secretdata
	testCS := []Test{
		{"Testing hostname", fmt.Sprintf("%s.%s.svc.cluster.local", runtime.GetName(), runtime.GetNamespace()), string(secret.Data["hostname"])},
		{"Testing protocol", service.Provides.Protocol, string(secret.Data["protocol"])},
		{"Testing port", "8443", string(secret.Data["port"])},
		{"Testing context", strings.TrimPrefix(service.Provides.Context, "/"), string(secret.Data["context"])},
		{"Testing username", auth["username"], string(secret.Data["username"])},
		{"Testing password", auth["password"], string(secret.Data["password"])},
	}

	verifyTests(testCS, t)
}

func TestCustomizePodSpec(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	spec := appstacksv1beta1.RuntimeComponentSpec{
		ApplicationImage:    appImage,
		Service:             service,
		ResourceConstraints: resourceContraints,
		ReadinessProbe:      readinessProbe,
		LivenessProbe:       livenessProbe,
		VolumeMounts:        []corev1.VolumeMount{volumeMount},
		PullPolicy:          &pullPolicy,
		Env:                 env,
		EnvFrom:             envFrom,
		Volumes:             []corev1.Volume{volume},
	}
	pts, runtime := &corev1.PodTemplateSpec{}, createRuntimeComponent(name, namespace, spec, labels, annotations)
	// else cond
	CustomizePodSpec(pts, runtime)
	noCont := len(pts.Spec.Containers)
	noPorts := len(pts.Spec.Containers[0].Ports)
	ptsSAN := pts.Spec.ServiceAccountName
	// if cond
	spec = appstacksv1beta1.RuntimeComponentSpec{
		ApplicationImage: appImage,
		Service: &appstacksv1beta1.RuntimeComponentService{
			Type:       &serviceType,
			Port:       8443,
			TargetPort: &targetPort,
		},
		ResourceConstraints: resourceContraints,
		ReadinessProbe:      readinessProbe,
		LivenessProbe:       livenessProbe,
		VolumeMounts:        []corev1.VolumeMount{volumeMount},
		PullPolicy:          &pullPolicy,
		Env:                 env,
		EnvFrom:             envFrom,
		Volumes:             []corev1.Volume{volume},
		Architecture:        arch,
		ServiceAccountName:  &serviceAccountName,
	}
	runtime = createRuntimeComponent(name, namespace, spec, labels, annotations)
	CustomizePodSpec(pts, runtime)
	ptsCSAN := pts.Spec.ServiceAccountName

	// affinity tests
	affArchs := pts.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions[0].Values[0]
	weight := pts.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Weight
	prefAffArchs := pts.Spec.Affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution[0].Preference.MatchExpressions[0].Values[0]
	assignedTPort := pts.Spec.Containers[0].Ports[0].ContainerPort

	testCPS := []Test{
		{"No containers", 1, noCont},
		{"No port", 1, noPorts},
		{"No ServiceAccountName", name, ptsSAN},
		{"ServiceAccountName available", serviceAccountName, ptsCSAN},
	}
	verifyTests(testCPS, t)

	testCA := []Test{
		{"Archs", arch[0], affArchs},
		{"Weight", int32(1), int32(weight)},
		{"Archs", arch[0], prefAffArchs},
		{"No target port", targetPort, assignedTPort},
	}
	verifyTests(testCA, t)

	mySecret := "my-secret"

	// Test service certificate
	certificate := v1beta1.Certificate{SecretName: mySecret}
	spec = appstacksv1beta1.RuntimeComponentSpec{Service: service}
	spec.Service.Certificate = &certificate
	runtime = createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizePodSpec(pts, runtime)

	spec = appstacksv1beta1.RuntimeComponentSpec{Service: service}
	spec.Service.CertificateSecretRef = &mySecret
	runtime = createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizePodSpec(pts, runtime)

}

func TestCustomizeServiceBinding(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	runtime := createRuntimeComponent(name, namespace, spec, labels, annotations)
	resolvedBindings := []string{"binding1", "binding2"}
	runtime.GetStatus().SetResolvedBindings(resolvedBindings)
	secret := &corev1.Secret{}
	container := &corev1.Container{}
	containers := []corev1.Container{*container}
	ps := &corev1.PodSpec{Containers: containers}

	CustomizeServiceBinding(secret, ps, runtime)

	testCS := []Test{
		{"Pod Spec EnvFrom", "binding1", ps.Containers[0].EnvFrom[0].SecretRef.Name},
		{"Pod Spec Env", secret.ResourceVersion, ps.Containers[0].Env[0].Value},
		{"Pod Spec Env", "RESOLVED_BINDING_SECRET_REV", ps.Containers[0].Env[0].Name},
	}

	verifyTests(testCS, t)
}

func TestCustomizeConsumedServices(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	runtime := createRuntimeComponent(name, namespace, spec, labels, annotations)
	consumed := []string{"consumed1", namespace + "-consumes"}
	svcbindingCategory := common.ConsumedServices{common.ServiceBindingCategoryOpenAPI: consumed}
	runtime.GetStatus().SetConsumedServices(svcbindingCategory)
	container := &corev1.Container{}
	containers := []corev1.Container{*container}
	ps := &corev1.PodSpec{Containers: containers}

	CustomizeConsumedServices(ps, runtime)

	testCS := []Test{
		{"Pod Spec Env Name", "CONSUMES_USERNAME", ps.Containers[0].Env[0].Name},
		{"Pod Spec Env SecretKeyRef Key", "username", ps.Containers[0].Env[0].ValueFrom.SecretKeyRef.Key},
		{"Pod Spec Env SecretKeyRef Option", true, *ps.Containers[0].Env[0].ValueFrom.SecretKeyRef.Optional},
	}

	verifyTests(testCS, t)

	runtime.Spec.Service.Consumes[0].MountPath = "myPath"
	CustomizeConsumedServices(ps, runtime)

	testCS = []Test{
		{"Pod Spec Volume Mounts Name", ps.Containers[0].VolumeMounts[0].Name, "runtime-consumes"},
		{"Pod Spec Volume Mounts Mount Path", ps.Containers[0].VolumeMounts[0].MountPath, "myPath//consumes"},
		{"Pod Spec Volume Mounts Read Only", ps.Containers[0].VolumeMounts[0].ReadOnly, true},
		{"Pod Spec Volume Name", ps.Volumes[0].Name, "runtime-consumes"},
		{"Pod Spec Volume Mounts Mount Path", ps.Volumes[0].VolumeSource.Secret.SecretName, "runtime-consumes"},
	}

	verifyTests(testCS, t)
}

func TestCustomizePersistence(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	spec := appstacksv1beta1.RuntimeComponentSpec{Storage: &storage}
	statefulSet, runtime := &appsv1.StatefulSet{}, createRuntimeComponent(name, namespace, spec, labels, annotations)
	statefulSet.Spec.Template.Spec.Containers = []corev1.Container{{}}
	statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{}
	// if vct == 0, runtimeVCT != nil, not found
	CustomizePersistence(statefulSet, runtime)
	ssK := statefulSet.Spec.VolumeClaimTemplates[0].TypeMeta.Kind
	ssMountPath := statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath

	//reset
	storageNilVCT := appstacksv1beta1.RuntimeComponentStorage{Size: "10Mi", MountPath: "/mnt/data", VolumeClaimTemplate: nil}
	spec = appstacksv1beta1.RuntimeComponentSpec{Storage: &storageNilVCT}
	statefulSet, runtime = &appsv1.StatefulSet{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	statefulSet.Spec.Template.Spec.Containers = []corev1.Container{{}}
	statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts, volumeMount)
	//runtimeVCT == nil, found
	CustomizePersistence(statefulSet, runtime)
	ssVolumeMountName := statefulSet.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name
	size := statefulSet.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
	testCP := []Test{
		{"Persistence kind with VCT", volumeCT.TypeMeta.Kind, ssK},
		{"PVC size", storage.Size, size.String()},
		{"Mount path", storage.MountPath, ssMountPath},
		{"Volume Mount Name", volumeCT.Name, ssVolumeMountName},
	}
	verifyTests(testCP, t)
}

func TestCustomizeServiceAccount(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	spec := appstacksv1beta1.RuntimeComponentSpec{PullSecret: &pullSecret}
	sa, runtime := &corev1.ServiceAccount{}, createRuntimeComponent(name, namespace, spec, labels, annotations)
	CustomizeServiceAccount(sa, runtime)
	emptySAIPS := sa.ImagePullSecrets[0].Name

	newSecret := "my-new-secret"
	spec = appstacksv1beta1.RuntimeComponentSpec{PullSecret: &newSecret}
	runtime = createRuntimeComponent(name, namespace, spec, labels, annotations)
	CustomizeServiceAccount(sa, runtime)

	testCSA := []Test{
		{"ServiceAccount image pull secrets is empty", pullSecret, emptySAIPS},
		{"ServiceAccount image pull secrets", newSecret, sa.ImagePullSecrets[0].Name},
	}
	verifyTests(testCSA, t)
}

func TestCustomizeKnativeService(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	spec := appstacksv1beta1.RuntimeComponentSpec{
		ApplicationImage: appImage,
		Service:          service,
		LivenessProbe:    livenessProbe,
		ReadinessProbe:   readinessProbe,
		PullPolicy:       &pullPolicy,
		Env:              env,
		EnvFrom:          envFrom,
		Volumes:          []corev1.Volume{volume},
	}
	ksvc, runtime := &servingv1alpha1.Service{}, createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeKnativeService(ksvc, runtime)
	ksvcNumPorts := len(ksvc.Spec.Template.Spec.Containers[0].Ports)
	ksvcSAN := ksvc.Spec.Template.Spec.ServiceAccountName

	ksvcLPPort := ksvc.Spec.Template.Spec.Containers[0].LivenessProbe.HTTPGet.Port
	ksvcLPTCP := ksvc.Spec.Template.Spec.Containers[0].LivenessProbe.TCPSocket.Port
	ksvcRPPort := ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe.HTTPGet.Port
	ksvcRPTCP := ksvc.Spec.Template.Spec.Containers[0].ReadinessProbe.TCPSocket.Port
	ksvcLabelNoExpose := ksvc.Labels["serving.knative.dev/visibility"]

	spec = appstacksv1beta1.RuntimeComponentSpec{
		ApplicationImage:   appImage,
		Service:            service,
		PullPolicy:         &pullPolicy,
		Env:                env,
		EnvFrom:            envFrom,
		Volumes:            []corev1.Volume{volume},
		ServiceAccountName: &serviceAccountName,
		LivenessProbe:      livenessProbe,
		ReadinessProbe:     readinessProbe,
		Expose:             &expose,
	}
	runtime = createRuntimeComponent(name, namespace, spec, labels, annotations)
	CustomizeKnativeService(ksvc, runtime)
	ksvcLabelTrueExpose := ksvc.Labels["serving.knative.dev/visibility"]

	fls := false
	runtime.Spec.Expose = &fls
	CustomizeKnativeService(ksvc, runtime)
	ksvcLabelFalseExpose := ksvc.Labels["serving.knative.dev/visibility"]

	testCKS := []Test{
		{"ksvc container ports", 1, ksvcNumPorts},
		{"ksvc ServiceAccountName is nil", name, ksvcSAN},
		{"ksvc ServiceAccountName not nil", *runtime.Spec.ServiceAccountName, ksvc.Spec.Template.Spec.ServiceAccountName},
		{"liveness probe port", intstr.IntOrString{}, ksvcLPPort},
		{"liveness probe TCP socket port", intstr.IntOrString{}, ksvcLPTCP},
		{"Readiness probe port", intstr.IntOrString{}, ksvcRPPort},
		{"Readiness probe TCP socket port", intstr.IntOrString{}, ksvcRPTCP},
		{"expose not set", "cluster-local", ksvcLabelNoExpose},
		{"expose set to true", "", ksvcLabelTrueExpose},
		{"expose set to false", "cluster-local", ksvcLabelFalseExpose},
	}
	verifyTests(testCKS, t)
}

func TestCustomizeHPA(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	spec := appstacksv1beta1.RuntimeComponentSpec{Autoscaling: autoscaling}
	hpa, runtime := &autoscalingv1.HorizontalPodAutoscaler{}, createRuntimeComponent(name, namespace, spec, labels, annotations)
	CustomizeHPA(hpa, runtime)
	nilSTRKind := hpa.Spec.ScaleTargetRef.Kind

	spec = appstacksv1beta1.RuntimeComponentSpec{Autoscaling: autoscaling, Storage: &storage}
	runtime = createRuntimeComponent(name, namespace, spec, labels, annotations)
	CustomizeHPA(hpa, runtime)
	STRKind := hpa.Spec.ScaleTargetRef.Kind

	testCHPA := []Test{
		{"Max replicas", autoscaling.MaxReplicas, hpa.Spec.MaxReplicas},
		{"Min replicas", *autoscaling.MinReplicas, *hpa.Spec.MinReplicas},
		{"Target CPU utilization", *autoscaling.TargetCPUUtilizationPercentage, *hpa.Spec.TargetCPUUtilizationPercentage},
		{"", name, hpa.Spec.ScaleTargetRef.Name},
		{"", "apps/v1", hpa.Spec.ScaleTargetRef.APIVersion},
		{"Storage not enabled", "Deployment", nilSTRKind},
		{"Storage enabled", "StatefulSet", STRKind},
	}
	verifyTests(testCHPA, t)
}

func TestValidate(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	storage2 := &appstacksv1beta1.RuntimeComponentStorage{}
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service, Storage: storage2}
	runtime := createRuntimeComponent(name, namespace, spec, labels, annotations)

	result, err := Validate(runtime)

	testCS := []Test{
		{"Error response", false, result},
		{"Error response", errors.New("validation failed: must set the field(s): spec.storage.size"), err},
	}

	verifyTests(testCS, t)

	storage2 = &appstacksv1beta1.RuntimeComponentStorage{Size: "size"}
	runtime.Spec.Storage = storage2

	result, err = Validate(runtime)

	testCS = []Test{
		{"Error response", false, result},
		{"Error response", errors.New("validation failed: cannot parse 'size': quantities must match the regular expression '^([+-]?[0-9.]+)([eEinumkKMGTP]*[-+]?[0-9]*)$'"), err},
	}

	verifyTests(testCS, t)

	runtime.Spec.Storage = &storage

	result, err = Validate(runtime)

	testCS = []Test{
		{"Result", true, result},
		{"Error response", nil, err},
	}

	verifyTests(testCS, t)
}

func TestCreateValidationError(t *testing.T) {
	result := createValidationError("Test Error")

	testCS := []Test{
		{"Validation error message", errors.New("validation failed: Test Error"), result},
	}

	verifyTests(testCS, t)
}

func TestCustomizeServiceMonitor(t *testing.T) {

	logf.SetLogger(logf.ZapLogger(true))
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}

	params := map[string][]string{
		"params": []string{"param1", "param2"},
	}

	portValue := intstr.FromString("web")
	// Endpoint for runtime
	endpointApp := &prometheusv1.Endpoint{
		Port:            "web",
		Scheme:          "myScheme",
		Interval:        "myInterval",
		Path:            "myPath",
		TLSConfig:       &prometheusv1.TLSConfig{},
		BasicAuth:       &prometheusv1.BasicAuth{},
		Params:          params,
		ScrapeTimeout:   "myScrapeTimeout",
		BearerTokenFile: "myBearerTokenFile",
		TargetPort:      &portValue,
	}
	endpointsApp := make([]prometheusv1.Endpoint, 1)
	endpointsApp[0] = *endpointApp

	// Endpoint for sm
	endpointsSM := make([]prometheusv1.Endpoint, 0)

	labelMap := map[string]string{"app": "my-app"}
	selector := &metav1.LabelSelector{MatchLabels: labelMap}
	smspec := &prometheusv1.ServiceMonitorSpec{Endpoints: endpointsSM, Selector: *selector}

	sm, runtime := &prometheusv1.ServiceMonitor{Spec: *smspec}, createRuntimeComponent(name, namespace, spec, labels, annotations)
	runtime.Spec.Monitoring = &appstacksv1beta1.RuntimeComponentMonitoring{Labels: labelMap, Endpoints: endpointsApp}

	CustomizeServiceMonitor(sm, runtime)

	labelMatches := map[string]string{
		"monitor.app.stacks/enabled": "true",
		"app.kubernetes.io/instance": name,
	}

	allSMLabels := runtime.GetLabels()
	for key, value := range runtime.Spec.Monitoring.Labels {
		allSMLabels[key] = value
	}

	// Expected values
	appPort := runtime.Spec.Monitoring.Endpoints[0].Port
	appScheme := runtime.Spec.Monitoring.Endpoints[0].Scheme
	appInterval := runtime.Spec.Monitoring.Endpoints[0].Interval
	appPath := runtime.Spec.Monitoring.Endpoints[0].Path
	appTLSConfig := runtime.Spec.Monitoring.Endpoints[0].TLSConfig
	appBasicAuth := runtime.Spec.Monitoring.Endpoints[0].BasicAuth
	appParams := runtime.Spec.Monitoring.Endpoints[0].Params
	appScrapeTimeout := runtime.Spec.Monitoring.Endpoints[0].ScrapeTimeout
	appBearerTokenFile := runtime.Spec.Monitoring.Endpoints[0].BearerTokenFile

	testSM := []Test{
		{"Service Monitor label for app.kubernetes.io/instance", name, sm.Labels["app.kubernetes.io/instance"]},
		{"Service Monitor selector match labels", labelMatches, sm.Spec.Selector.MatchLabels},
		{"Service Monitor endpoints port", appPort, sm.Spec.Endpoints[0].Port},
		{"Service Monitor all labels", allSMLabels, sm.Labels},
		{"Service Monitor endpoints scheme", appScheme, sm.Spec.Endpoints[0].Scheme},
		{"Service Monitor endpoints interval", appInterval, sm.Spec.Endpoints[0].Interval},
		{"Service Monitor endpoints path", appPath, sm.Spec.Endpoints[0].Path},
		{"Service Monitor endpoints TLSConfig", appTLSConfig, sm.Spec.Endpoints[0].TLSConfig},
		{"Service Monitor endpoints basicAuth", appBasicAuth, sm.Spec.Endpoints[0].BasicAuth},
		{"Service Monitor endpoints params", appParams, sm.Spec.Endpoints[0].Params},
		{"Service Monitor endpoints scrapeTimeout", appScrapeTimeout, sm.Spec.Endpoints[0].ScrapeTimeout},
		{"Service Monitor endpoints bearerTokenFile", appBearerTokenFile, sm.Spec.Endpoints[0].BearerTokenFile},
	}

	verifyTests(testSM, t)

}

func TestGetCondition(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	status := &appstacksv1beta1.RuntimeComponentStatus{
		Conditions: []appstacksv1beta1.StatusCondition{
			{
				Status: corev1.ConditionTrue,
				Type:   appstacksv1beta1.StatusConditionTypeReconciled,
			},
		},
	}
	conditionType := appstacksv1beta1.StatusConditionTypeReconciled
	cond := GetCondition(conditionType, status)
	testGC := []Test{{"Set status condition", status.Conditions[0].Status, cond.Status}}
	verifyTests(testGC, t)

	status = &appstacksv1beta1.RuntimeComponentStatus{}
	cond = GetCondition(conditionType, status)
	testGC = []Test{{"Set status condition", 0, len(status.Conditions)}}
	verifyTests(testGC, t)
}

func TestSetCondition(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	status := &appstacksv1beta1.RuntimeComponentStatus{
		Conditions: []appstacksv1beta1.StatusCondition{
			{Type: appstacksv1beta1.StatusConditionTypeReconciled},
		},
	}
	condition := appstacksv1beta1.StatusCondition{
		Status: corev1.ConditionTrue,
		Type:   appstacksv1beta1.StatusConditionTypeReconciled,
	}
	SetCondition(condition, status)
	testSC := []Test{{"Set status condition", condition.Status, status.Conditions[0].Status}}
	verifyTests(testSC, t)
}

func TestGetWatchNamespaces(t *testing.T) {
	// Set the logger to development mode for verbose logs
	logf.SetLogger(logf.ZapLogger(true))

	os.Setenv("WATCH_NAMESPACE", "")
	namespaces, err := GetWatchNamespaces()
	configMapConstTests := []Test{
		{"namespaces", []string{""}, namespaces},
		{"error", nil, err},
	}
	verifyTests(configMapConstTests, t)

	os.Setenv("WATCH_NAMESPACE", "ns1")
	namespaces, err = GetWatchNamespaces()
	configMapConstTests = []Test{
		{"namespaces", []string{"ns1"}, namespaces},
		{"error", nil, err},
	}
	verifyTests(configMapConstTests, t)

	os.Setenv("WATCH_NAMESPACE", "ns1,ns2,ns3")
	namespaces, err = GetWatchNamespaces()
	configMapConstTests = []Test{
		{"namespaces", []string{"ns1", "ns2", "ns3"}, namespaces},
		{"error", nil, err},
	}
	verifyTests(configMapConstTests, t)

	os.Setenv("WATCH_NAMESPACE", " ns1   ,  ns2,  ns3  ")
	namespaces, err = GetWatchNamespaces()
	configMapConstTests = []Test{
		{"namespaces", []string{"ns1", "ns2", "ns3"}, namespaces},
		{"error", nil, err},
	}
	verifyTests(configMapConstTests, t)
}

func TestUpdateAppDefinition(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service, Version: "v1alpha"}
	app := createRuntimeComponent(name, namespace, spec, labels, annotations)

	// Toggle app definition off [disabled]
	enabled := false
	app.Spec.CreateAppDefinition = &enabled
	labels, annotations := createAppDefinitionTags(app)
	UpdateAppDefinition(labels, annotations, app)

	appDefinitionTests := []Test{
		{"Label unset", 0, len(labels)},
		{"Annotation unset", 0, len(annotations)},
	}

	verifyTests(appDefinitionTests, t)

	// Toggle back on [active]
	enabled = true
	completeLabels, completeAnnotations := createAppDefinitionTags(app)
	UpdateAppDefinition(labels, annotations, app)

	appDefinitionTests = []Test{
		{"Label set", labels["kappnav.app.auto-create"], completeLabels["kappnav.app.auto-create"]},
		{"Annotation name set", annotations["kappnav.app.auto-create.name"], completeAnnotations["kappnav.app.auto-create.name"]},
		{"Annotation kinds set", annotations["kappnav.app.auto-create.kinds"], completeAnnotations["kappnav.app.auto-create.kinds"]},
		{"Annotation label set", annotations["kappnav.app.auto-create.label"], completeAnnotations["kappnav.app.auto-create.label"]},
		{"Annotation labels-values", annotations["kappnav.app.auto-create.labels-values"], completeAnnotations["kappnav.app.auto-create.labels-values"]},
		{"Annotation version set", annotations["kappnav.app.auto-create.version"], completeAnnotations["kappnav.app.auto-create.version"]},
	}
	verifyTests(appDefinitionTests, t)

	// Verify labels are still set when CreateApp is undefined [default]
	app.Spec.CreateAppDefinition = nil
	UpdateAppDefinition(labels, annotations, app)

	appDefinitionTests = []Test{
		{"Label set", labels["kappnav.app.auto-create"], completeLabels["kappnav.app.auto-create"]},
		{"Annotation name set", annotations["kappnav.app.auto-create.name"], completeAnnotations["kappnav.app.auto-create.name"]},
		{"Annotation kinds set", annotations["kappnav.app.auto-create.kinds"], completeAnnotations["kappnav.app.auto-create.kinds"]},
		{"Annotation label set", annotations["kappnav.app.auto-create.label"], completeAnnotations["kappnav.app.auto-create.label"]},
		{"Annotation labels-values", annotations["kappnav.app.auto-create.labels-values"], completeAnnotations["kappnav.app.auto-create.labels-values"]},
		{"Annotation version set", annotations["kappnav.app.auto-create.version"], completeAnnotations["kappnav.app.auto-create.version"]},
	}
	verifyTests(appDefinitionTests, t)

	// Verify labels are still set when CreateApp is undefined [default]
	app.Spec.Version = ""
	UpdateAppDefinition(labels, annotations, app)

	appDefinitionTests = []Test{
		{"Label set", labels["kappnav.app.auto-create"], completeLabels["kappnav.app.auto-create"]},
		{"Annotation name set", annotations["kappnav.app.auto-create.name"], completeAnnotations["kappnav.app.auto-create.name"]},
		{"Annotation kinds set", annotations["kappnav.app.auto-create.kinds"], completeAnnotations["kappnav.app.auto-create.kinds"]},
		{"Annotation label set", annotations["kappnav.app.auto-create.label"], completeAnnotations["kappnav.app.auto-create.label"]},
		{"Annotation labels-values", annotations["kappnav.app.auto-create.labels-values"], completeAnnotations["kappnav.app.auto-create.labels-values"]},
		{"Annotation version set", "", app.Annotations["kappnav.app.auto-create.version"]},
	}
	verifyTests(appDefinitionTests, t)

}

func TestContainsString(t *testing.T) {
	fullString := []string{"string1", "string2"}
	subString := ""

	result := ContainsString(fullString, subString)

	testCS := []Test{
		{"Testing when string is not present", false, result},
	}

	verifyTests(testCS, t)

	subString = "string2"
	result = ContainsString(fullString, subString)

	testCS = []Test{
		{"Testing when string is present", true, result},
	}

	verifyTests(testCS, t)
}

func TestAppendIfNotSubstring(t *testing.T) {
	listOfStrings := ""
	result := AppendIfNotSubstring("append", listOfStrings)

	testCS := []Test{
		{"No list of strings", "append", result},
	}

	verifyTests(testCS, t)

	listOfStrings = "string1,string2"
	result = AppendIfNotSubstring("append", listOfStrings)

	testCS = []Test{
		{"List of strings", listOfStrings + ",append", result},
	}

	verifyTests(testCS, t)
}

func TestGetConnectToAnnotation(t *testing.T) {
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	runtime := createRuntimeComponent(name, namespace, spec, labels, annotations)
	runtime.Spec.Service.Consumes[0].Namespace = namespace

	result := GetConnectToAnnotation(runtime)

	annos := map[string]string{
		"app.openshift.io/connects-to": "consumes",
	}
	testCS := []Test{
		{"Annotations", annos, result},
	}
	verifyTests(testCS, t)
}

func TestGetOpenShiftAnnotations(t *testing.T) {
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service}
	runtime := createRuntimeComponent(name, namespace, spec, labels, annotations)

	annos := map[string]string{
		"image.opencontainers.org/source": "source",
	}
	runtime.Annotations = annos

	result := GetOpenShiftAnnotations(runtime)

	annos = map[string]string{
		"app.openshift.io/vcs-uri": "source",
	}
	testCS := []Test{
		{"OpenShiftAnnotations", annos["app.openshift.io/vcs-uri"], result["app.openshift.io/vcs-uri"]},
	}

	verifyTests(testCS, t)
}

func TestIsClusterWide(t *testing.T) {
	namespaces := []string{"namespace"}
	result := IsClusterWide(namespaces)

	testCS := []Test{
		{"One namespace", false, result},
	}

	verifyTests(testCS, t)

	namespaces = []string{""}
	result = IsClusterWide(namespaces)

	testCS = []Test{
		{"All namespaces", true, result},
	}

	verifyTests(testCS, t)

	namespaces = []string{"namespace1", "namespace2"}
	result = IsClusterWide(namespaces)

	testCS = []Test{
		{"Two namespaces", false, result},
	}

	verifyTests(testCS, t)
}

func TestCustomizeIngress(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	ing := networkingv1beta1.Ingress{}
	cert := v1beta1.Certificate{}
	certSecretRef := "my-ref"
	route := appstacksv1beta1.RuntimeComponentRoute{Host: "routeHost", Path: "myPath"}
	spec := appstacksv1beta1.RuntimeComponentSpec{Service: service, Route: &route}
	runtime := createRuntimeComponent(name, namespace, spec, labels, annotations)

	CustomizeIngress(&ing, runtime)

	testCS := []Test{
		{"Ingress Labels", labels["key1"], ing.Labels["key1"]},
		{"Ingress Annotations", annotations, ing.Annotations},
		{"Ingress Route Host", "routeHost", ing.Spec.Rules[0].Host},
		{"Ingress Route Path", "myPath", ing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Path},
		{"Ingress Route ServiceName", name, ing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.ServiceName},
		{"Ingress Route ServicePort", "myservice", ing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Backend.ServicePort.StrVal},
		{"Ingress TLS", 0, len(ing.Spec.TLS)},
	}

	verifyTests(testCS, t)

	route = appstacksv1beta1.RuntimeComponentRoute{Host: "routeHost", Path: "myPath", Certificate: &cert}
	runtime.Spec.Route = &route

	CustomizeIngress(&ing, runtime)

	testCS = []Test{
		{"Ingress TLS Host", "routeHost", ing.Spec.TLS[0].Hosts[0]},
		{"Ingress TLS SecretName", name + "-route-tls", ing.Spec.TLS[0].SecretName},
	}

	verifyTests(testCS, t)

	cert = v1beta1.Certificate{SecretName: "my-secret"}
	route = appstacksv1beta1.RuntimeComponentRoute{Host: "routeHost", Path: "myPath", Certificate: &cert}

	CustomizeIngress(&ing, runtime)

	testCS = []Test{
		{"Ingress TLS SecretName", name + "my-secret", ing.Spec.TLS[0].SecretName},
	}

	verifyTests(testCS, t)

	route = appstacksv1beta1.RuntimeComponentRoute{Host: "routeHost", Path: "myPath", CertificateSecretRef: &certSecretRef}

	CustomizeIngress(&ing, runtime)

	testCS = []Test{
		{"Ingress TLS SecretName", certSecretRef, ing.Spec.TLS[0].SecretName},
	}

	verifyTests(testCS, t)
}

// Helper Functions
// Unconditionally set the proper tags for an enabled runtime omponent
func createAppDefinitionTags(app *appstacksv1beta1.RuntimeComponent) (map[string]string, map[string]string) {
	// The purpose of this function demands all fields configured
	if app.Spec.Version == "" {
		app.Spec.Version = "v1alpha"
	}
	// set fields
	label := map[string]string{
		"kappnav.app.auto-create": "true",
	}
	annotations := map[string]string{
		"kappnav.app.auto-create.name":          app.Spec.ApplicationName,
		"kappnav.app.auto-create.kinds":         "Deployment, StatefulSet, Service, Route, Ingress, ConfigMap",
		"kappnav.app.auto-create.label":         "app.kubernetes.io/part-of",
		"kappnav.app.auto-create.labels-values": app.Spec.ApplicationName,
		"kappnav.app.auto-create.version":       app.Spec.Version,
	}
	return label, annotations
}
func createRuntimeComponent(n, ns string, spec appstacksv1beta1.RuntimeComponentSpec, labels map[string]string, annotations map[string]string) *appstacksv1beta1.RuntimeComponent {
	app := &appstacksv1beta1.RuntimeComponent{
		ObjectMeta: metav1.ObjectMeta{Name: n, Namespace: ns, Labels: labels, Annotations: annotations},
		Spec:       spec,
	}
	return app
}

func verifyTests(tests []Test, t *testing.T) {
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.actual, tt.expected) {
			t.Errorf("%s test expected: (%v) actual: (%v)", tt.test, tt.expected, tt.actual)
		}
	}
}
