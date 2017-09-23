package acceptance_test

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	acceptance "github.com/cloudfoundry/bosh-bootloader/acceptance-tests"

	"github.com/cloudfoundry/bosh-bootloader/acceptance-tests/actors"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("concourse deployment test", func() {
	var (
		bbl           actors.BBL
		state         acceptance.State
		lbURL         string
		configuration acceptance.Config
		boshCLI       actors.BOSHCLI
		sshSession    *gexec.Session
		username      string
		password      string
		address       string
		caCertPath    string
	)

	BeforeEach(func() {
		var err error
		configuration, err = acceptance.LoadConfig()
		Expect(err).NotTo(HaveOccurred())

		bbl = actors.NewBBL("/var/folders/73/sl68xpyd2dggdz6cs2t48f6w0000gn/T/239084616", pathToBBL, configuration, "concourse-env")
		state = acceptance.NewState(configuration.StateFileDir)

		//session := bbl.Up("--name", bbl.PredefinedEnvID())
		//Eventually(session, 40*time.Minute).Should(gexec.Exit(0))

		//certPath, err := testhelpers.WriteContentsToTempFile(testhelpers.BBL_CERT)
		//Expect(err).NotTo(HaveOccurred())

		//keyPath, err := testhelpers.WriteContentsToTempFile(testhelpers.BBL_KEY)
		//Expect(err).NotTo(HaveOccurred())

		//session = bbl.CreateLB("concourse", certPath, keyPath, "")
		//Eventually(session, 10*time.Minute).Should(gexec.Exit(0))

		//lbURL, err = actors.LBURL(configuration, bbl, state)
		//Expect(err).NotTo(HaveOccurred())

		boshCLI = actors.NewBOSHCLI()

		username = bbl.DirectorUsername()
		password = bbl.DirectorPassword()
		address = bbl.DirectorAddress()
		caCertPath = bbl.DirectorCACert()
	})

	AfterEach(func() {
		if sshSession != nil {
			//boshCLI.DeleteDeployment(address, caCertPath, username, password, "concourse")
			sshSession.Interrupt()
			Eventually(sshSession, "5s").Should(gexec.Exit())
		}
		//session := bbl.Destroy()
		//Eventually(session, 10*time.Minute).Should(gexec.Exit())
	})

	It("is able to deploy concourse and teardown infrastructure", func() {
		By("creating an ssh tunnel to the director in print-env", func() {
			sshSession = bbl.StartSSHTunnel()
		})

		By("uploading releases and stemcells", func() {
			fmt.Println("*** config  ***")
			fmt.Println(configuration.ConcourseReleasePath)
			fmt.Println("*** env var ***")
			fmt.Println(os.Getenv("CONCOURSE_RELEASE_PATH"))
			fmt.Println("***   fin   ***")
			err := boshCLI.UploadRelease(address, caCertPath, username, password, configuration.ConcourseReleasePath)
			Expect(err).NotTo(HaveOccurred())

			err = boshCLI.UploadRelease(address, caCertPath, username, password, configuration.GardenReleasePath)
			Expect(err).NotTo(HaveOccurred())

			err = boshCLI.UploadStemcell(address, caCertPath, username, password, configuration.StemcellPath)
			Expect(err).NotTo(HaveOccurred())
		})

		By("running bosh deploy and checking all the vms are running", func() {
			err := boshCLI.Deploy(address, caCertPath, username, password, "concourse",
				fmt.Sprintf("%s/concourse-deployment.yml", configuration.ConcourseDeploymentPath),
				"concourse-vars.yml",
				[]string{fmt.Sprintf("%s/operations/%s.yml", configuration.ConcourseDeploymentPath, configuration.IAAS)},
				map[string]string{"domain": lbURL},
			)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() ([]actors.BOSHVM, error) {
				vms, err := boshCLI.VMs(address, caCertPath, username, password, "concourse")
				if err != nil {
					return []actors.BOSHVM{}, err
				}

				vmsNoID := []actors.BOSHVM{}
				for _, vm := range vms {
					vm.ID = ""
					vm.IPs = nil
					vmsNoID = append(vmsNoID, vm)
				}
				return vmsNoID, nil
			}, "1m", "10s").Should(ConsistOf([]actors.BOSHVM{
				{JobName: "worker", Index: 0, State: "running"},
				{JobName: "worker", Index: 1, State: "running"},
				{JobName: "db", Index: 0, State: "running"},
				{JobName: "web", Index: 0, State: "running"},
			}))
		})

		By("testing the deployment", func() {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			client := &http.Client{Transport: tr}

			resp, err := client.Get(lbURL)
			Expect(err).NotTo(HaveOccurred())

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(body)).To(ContainSubstring("<title>Concourse</title>"))
		})

		By("deleting the deployment", func() {
			err := boshCLI.DeleteDeployment(address, caCertPath, username, password, "concourse")
			Expect(err).NotTo(HaveOccurred())
		})

		By("deleting load balancers", func() {
			session := bbl.DeleteLBs()
			Eventually(session, 15*time.Minute).Should(gexec.Exit(0))
		})
	})
})
