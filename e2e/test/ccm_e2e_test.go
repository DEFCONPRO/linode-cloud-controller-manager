package test

import (
	"e2e_test/test/framework"
	"log"
	"strings"

	"github.com/appscode/go/wait"
	"github.com/codeskyblue/go-sh"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("CloudControllerManager", func() {
	var (
		err     error
		f       *framework.Invocation
		workers []string
	)

	const (
		annLinodeLoadBalancerTLS = "service.beta.kubernetes.io/linode-loadbalancer-tls"
		annLinodeProtocol        = "service.beta.kubernetes.io/linode-loadbalancer-protocol"
	)

	BeforeEach(func() {
		f = root.Invoke()
		workers, err = f.GetNodeList()
		Expect(err).NotTo(HaveOccurred())
		Expect(len(workers)).Should(BeNumerically(">=", 2))
	})

	var createPodWithLabel = func(pods []string, ports []core.ContainerPort, image string, labels map[string]string, selectNode bool) {
		for i, pod := range pods {
			p := f.LoadBalancer.GetPodObject(pod, image, ports, labels)
			if selectNode {
				p = f.LoadBalancer.SetNodeSelector(p, workers[i])
			}
			err = f.LoadBalancer.CreatePod(p)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	var deletePods = func(pods []string) {
		for _, pod := range pods {
			err = f.LoadBalancer.DeletePod(pod)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	var deleteService = func() {
		err = f.LoadBalancer.DeleteService()
		Expect(err).NotTo(HaveOccurred())
	}

	var deleteSecret = func(name string) {
		err = f.LoadBalancer.DeleteSecret(name)
		Expect(err).NotTo(HaveOccurred())
	}

	var createServiceWithSelector = func(selector map[string]string, ports []core.ServicePort) {
		err = f.LoadBalancer.CreateService(selector, nil, ports)
		Expect(err).NotTo(HaveOccurred())
	}

	var createServiceWithAnnotations = func(labels map[string]string, annotations map[string]string, ports []core.ServicePort) {
		err = f.LoadBalancer.CreateService(labels, annotations, ports)
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Test", func() {
		Context("Simple", func() {
			Context("Load Balancer", func() {
				var (
					pods   []string
					labels map[string]string
				)

				BeforeEach(func() {
					pods = []string{"test-pod-1", "test-pod-2"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}
					labels = map[string]string{
						"app": "test-loadbalancer",
					}

					By("Creating Pods")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, true)

					By("Creating Service")
					createServiceWithSelector(labels, servicePorts)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should reach all pods", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					var counter1, counter2 int

					By("Waiting for Response from the LoadBalancer url: " + eps[0])
					err = wait.PollImmediate(framework.RetryInterval, framework.RetryTimout, func() (bool, error) {
						resp, err := sh.Command("curl", "--max-time", "5", "-s", eps[0]).Output()
						if err != nil {
							return false, nil
						}
						stringResp := string(resp)
						if strings.Contains(stringResp, pods[0]) {
							log.Println("Got response from " + pods[0])
							counter1++
						} else if strings.Contains(stringResp, pods[1]) {
							log.Println("Got response from " + pods[1])
							counter2++
						}

						if counter1 > 0 && counter2 > 0 {
							return true, nil
						}
						return false, nil
					})
					Expect(counter1).Should(BeNumerically(">", 0))
					Expect(counter2).Should(BeNumerically(">", 0))
				})
			})
		})
	})

	Describe("Test", func() {
		Context("LoadBalancer", func() {
			Context("With single TLS port", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string
					secretName  string
				)
				BeforeEach(func() {
					pods = []string{"test-single-port-pod"}
					ports := []core.ContainerPort{
						{
							Name:          "https",
							ContainerPort: 8080,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "https",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
					}
					secretName = "tls-secret"
					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeLoadBalancerTLS: `[ { "tls-secret-name": "` + secretName + `", "port": 80} ]`,
						annLinodeProtocol:        "https",
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Secret")
					err = f.LoadBalancer.CreateTLSSecret("tls-secret")
					Expect(err).NotTo(HaveOccurred())

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts)
				})

				AfterEach(func() {
					By("Deleting the Secrets")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Deleting the Secret")
					deleteSecret(secretName)
				})

				It("should reach the pod via tls", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					By("Waiting for Response from the LoadBalancer url: " + eps[0])
					err = framework.WaitForHTTPSResponse(eps[0], pods[0])
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("With Multiple TLS Ports", func() {
				var (
					pods        []string
					labels      map[string]string
					annotations map[string]string
					secretName1 string
					secretName2 string
				)
				BeforeEach(func() {
					pods = []string{"tls-multi-port-pod"}
					secretName1 = "tls-secret-1"
					secretName2 = "tls-secret-2"
					labels = map[string]string{
						"app": "test-loadbalancer",
					}
					annotations = map[string]string{
						annLinodeLoadBalancerTLS: `[ { "tls-secret-name": "` + secretName1 + `", "port": 80},  {"tls-secret-name": "` + secretName2 + `", "port": 443}]`,
						annLinodeProtocol:        "https",
					}
					ports := []core.ContainerPort{
						{
							Name:          "https1",
							ContainerPort: 8080,
						},
						{
							Name:          "https2",
							ContainerPort: 8989,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "https-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
						{
							Name:       "https-2",
							Port:       443,
							TargetPort: intstr.FromInt(8989),
							Protocol:   "TCP",
						},
					}

					By("Creating Pod")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, false)

					By("Creating Secret")
					err = f.LoadBalancer.CreateTLSSecret(secretName1)
					Expect(err).NotTo(HaveOccurred())
					err = f.LoadBalancer.CreateTLSSecret(secretName2)
					Expect(err).NotTo(HaveOccurred())

					By("Creating Service")
					createServiceWithAnnotations(labels, annotations, servicePorts)
				})

				AfterEach(func() {
					By("Deleting the Secrets")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()

					By("Deleting the Secret")
					deleteSecret(secretName1)
					deleteSecret(secretName2)
				})

				It("should reach the pod via tls", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					By("Waiting for Response from the LoadBalancer urls: " + eps[0] + ", " + eps[1])
					for _, ep := range eps {
						err = framework.WaitForHTTPSResponse(ep, pods[0])
						Expect(err).NotTo(HaveOccurred())
					}
				})
			})

			Context("With Multiple HTTP Ports", func() {
				var (
					pods   []string
					labels map[string]string
				)

				BeforeEach(func() {
					pods = []string{"test-pod"}
					ports := []core.ContainerPort{
						{
							Name:          "http-1",
							ContainerPort: 8080,
						},
						{
							Name:          "http-2",
							ContainerPort: 8989,
						},
					}
					servicePorts := []core.ServicePort{
						{
							Name:       "http-1",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   "TCP",
						},
						{
							Name:       "http-2",
							Port:       8888,
							TargetPort: intstr.FromInt(8989),
							Protocol:   "TCP",
						},
					}
					labels = map[string]string{
						"app": "test-loadbalancer",
					}

					By("Creating Pods")
					createPodWithLabel(pods, ports, framework.TestServerImage, labels, true)

					By("Creating Service")
					createServiceWithSelector(labels, servicePorts)
				})

				AfterEach(func() {
					By("Deleting the Pods")
					deletePods(pods)

					By("Deleting the Service")
					deleteService()
				})

				It("should reach all pods", func() {
					By("Checking TCP Response")
					eps, err := f.LoadBalancer.GetHTTPEndpoints()
					Expect(err).NotTo(HaveOccurred())
					Expect(len(eps)).Should(BeNumerically(">=", 1))

					By("Waiting for Response from the LoadBalancer url: " + eps[0] + " " + eps[1])
					for _, ep := range eps {
						err = framework.WaitForHTTPResponse(ep, pods[0])
						Expect(err).NotTo(HaveOccurred())
					}
				})
			})
		})
	})
})