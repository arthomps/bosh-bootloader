package commands_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cloudfoundry/bosh-bootloader/commands"
	"github.com/cloudfoundry/bosh-bootloader/fakes"
	"github.com/cloudfoundry/bosh-bootloader/storage"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
)

var _ = Describe("AWS Create LBs", func() {
	Describe("Execute", func() {
		var (
			command              commands.AWSCreateLBs
			terraformManager     *fakes.TerraformManager
			cloudConfigManager   *fakes.CloudConfigManager
			stateStore           *fakes.StateStore
			environmentValidator *fakes.EnvironmentValidator
			incomingState        storage.State

			certPath  string
			keyPath   string
			chainPath string
		)

		BeforeEach(func() {
			terraformManager = &fakes.TerraformManager{}
			cloudConfigManager = &fakes.CloudConfigManager{}
			stateStore = &fakes.StateStore{}
			environmentValidator = &fakes.EnvironmentValidator{}

			incomingState = storage.State{}

			tempCertFile, err := ioutil.TempFile("", "cert")
			Expect(err).NotTo(HaveOccurred())
			certPath = tempCertFile.Name()
			err = ioutil.WriteFile(certPath, []byte("some-cert"), os.ModePerm)
			Expect(err).NotTo(HaveOccurred())

			tempKeyFile, err := ioutil.TempFile("", "key")
			Expect(err).NotTo(HaveOccurred())
			keyPath = tempKeyFile.Name()
			err = ioutil.WriteFile(keyPath, []byte("some-key"), os.ModePerm)
			Expect(err).NotTo(HaveOccurred())

			tempChainFile, err := ioutil.TempFile("", "chain")
			Expect(err).NotTo(HaveOccurred())
			chainPath = tempChainFile.Name()
			err = ioutil.WriteFile(chainPath, []byte("some-chain"), os.ModePerm)
			Expect(err).NotTo(HaveOccurred())

			command = commands.NewAWSCreateLBs(cloudConfigManager, stateStore, terraformManager, environmentValidator)
		})

		Context("when lb type desired is cf", func() {
			var (
				statePassedToTerraform     storage.State
				stateReturnedFromTerraform storage.State
			)
			BeforeEach(func() {
				statePassedToTerraform = storage.State{}
				statePassedToTerraform.LB = storage.LB{
					Type: "cf",
					Cert: "some-cert",
					Key:  "some-key",
				}

				stateReturnedFromTerraform = statePassedToTerraform
				terraformManager.ApplyCall.Returns.BBLState = stateReturnedFromTerraform
			})

			It("creates a load balancer with certificate using terraform", func() {
				err := command.Execute(
					commands.CreateLBsConfig{
						AWS: commands.AWSCreateLBsConfig{
							LBType:   "cf",
							CertPath: certPath,
							KeyPath:  keyPath,
						},
					},
					incomingState,
				)
				Expect(err).NotTo(HaveOccurred())

				Expect(terraformManager.InitCall.CallCount).To(Equal(1))
				Expect(terraformManager.InitCall.Receives.BBLState).To(Equal(statePassedToTerraform))

				Expect(terraformManager.ApplyCall.CallCount).To(Equal(1))
				Expect(terraformManager.ApplyCall.Receives.BBLState).To(Equal(statePassedToTerraform))

				Expect(stateStore.SetCall.Receives[1].State).To(Equal(stateReturnedFromTerraform))
				Expect(cloudConfigManager.InitializeCall.CallCount).To(Equal(1))
				Expect(cloudConfigManager.InitializeCall.Receives.State.LB.Type).To(Equal("cf"))
				Expect(cloudConfigManager.UpdateCall.CallCount).To(Equal(1))
				Expect(cloudConfigManager.UpdateCall.Receives.State.LB.Type).To(Equal("cf"))
			})

			Context("when the optional chain is provided", func() {
				BeforeEach(func() {
					statePassedToTerraform.LB.Chain = "some-chain"

					stateReturnedFromTerraform = statePassedToTerraform
					terraformManager.ApplyCall.Returns.BBLState = stateReturnedFromTerraform
				})

				It("creates a load balancer with certificate using terraform", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:    "cf",
								CertPath:  certPath,
								KeyPath:   keyPath,
								ChainPath: chainPath,
							},
						},
						incomingState,
					)
					Expect(err).NotTo(HaveOccurred())

					Expect(terraformManager.InitCall.Receives.BBLState).To(Equal(statePassedToTerraform))
					Expect(terraformManager.ApplyCall.Receives.BBLState).To(Equal(statePassedToTerraform))
					Expect(stateStore.SetCall.Receives[1].State).To(Equal(stateReturnedFromTerraform))
				})
			})

			Context("when a domain is provided", func() {
				BeforeEach(func() {
					statePassedToTerraform.LB = storage.LB{
						Type:   "cf",
						Cert:   "some-cert",
						Key:    "some-key",
						Domain: "some-domain",
					}

					stateReturnedFromTerraform = statePassedToTerraform
					terraformManager.ApplyCall.Returns.BBLState = stateReturnedFromTerraform
				})

				It("creates dns records for provided domain", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:   "cf",
								CertPath: certPath,
								KeyPath:  keyPath,
								Domain:   "some-domain",
							},
						},
						incomingState,
					)
					Expect(err).NotTo(HaveOccurred())

					Expect(terraformManager.InitCall.Receives.BBLState).To(Equal(statePassedToTerraform))
					Expect(terraformManager.ApplyCall.Receives.BBLState).To(Equal(statePassedToTerraform))
					Expect(stateStore.SetCall.Receives[1].State).To(Equal(stateReturnedFromTerraform))
				})
			})

			Context("when a domain exists", func() {
				BeforeEach(func() {
					incomingState.LB = storage.LB{
						Type:   "cf",
						Cert:   "some-cert",
						Key:    "some-key",
						Domain: "some-domain",
					}
					statePassedToTerraform = incomingState

					stateReturnedFromTerraform = statePassedToTerraform
					terraformManager.ApplyCall.Returns.BBLState = stateReturnedFromTerraform
				})

				It("does not change domain", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:   "cf",
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						incomingState,
					)
					Expect(err).NotTo(HaveOccurred())

					Expect(terraformManager.InitCall.Receives.BBLState).To(Equal(statePassedToTerraform))
					Expect(terraformManager.ApplyCall.Receives.BBLState).To(Equal(statePassedToTerraform))
					Expect(stateStore.SetCall.Receives[1].State).To(Equal(stateReturnedFromTerraform))
				})
			})

			Context("when lb type desired is concourse", func() {
				BeforeEach(func() {
					statePassedToTerraform = incomingState
					statePassedToTerraform.LB = storage.LB{
						Type: "concourse",
						Cert: "some-cert",
						Key:  "some-key",
					}

					stateReturnedFromTerraform = statePassedToTerraform
					terraformManager.ApplyCall.Returns.BBLState = stateReturnedFromTerraform
				})

				It("creates a load balancer with certificate using terraform", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:   "concourse",
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						incomingState,
					)
					Expect(err).NotTo(HaveOccurred())

					Expect(terraformManager.ApplyCall.Receives.BBLState).To(Equal(statePassedToTerraform))
					Expect(stateStore.SetCall.Receives[1].State).To(Equal(stateReturnedFromTerraform))
				})

				Context("when optional chain is provided", func() {
					BeforeEach(func() {
						statePassedToTerraform.LB.Chain = "some-chain"

						stateReturnedFromTerraform = statePassedToTerraform
						terraformManager.ApplyCall.Returns.BBLState = stateReturnedFromTerraform
					})

					It("creates a load balancer with certificate using terraform", func() {
						err := command.Execute(
							commands.CreateLBsConfig{
								AWS: commands.AWSCreateLBsConfig{
									LBType:    "concourse",
									CertPath:  certPath,
									KeyPath:   keyPath,
									ChainPath: chainPath,
								},
							},
							incomingState,
						)
						Expect(err).NotTo(HaveOccurred())

						Expect(terraformManager.ApplyCall.Receives.BBLState).To(Equal(statePassedToTerraform))
						Expect(stateStore.SetCall.Receives[1].State).To(Equal(stateReturnedFromTerraform))
					})
				})
			})
		})

		Context("when the bbl environment does not have a BOSH director", func() {
			It("does not call cloudConfigManager", func() {
				terraformManager.ApplyCall.Returns.BBLState = storage.State{
					NoDirector: true,
				}

				err := command.Execute(
					commands.CreateLBsConfig{
						AWS: commands.AWSCreateLBsConfig{
							LBType:   "concourse",
							CertPath: certPath,
							KeyPath:  keyPath,
						},
					},
					incomingState,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(cloudConfigManager.UpdateCall.CallCount).To(Equal(0))
			})
		})

		Context("when the environment validator fails", func() {
			BeforeEach(func() {
				environmentValidator.ValidateCall.Returns.Error = errors.New("environment not found")
			})

			It("returns an error", func() {
				err := command.Execute(
					commands.CreateLBsConfig{
						AWS: commands.AWSCreateLBsConfig{
							LBType:   "concourse",
							CertPath: certPath,
							KeyPath:  keyPath,
						},
					},
					incomingState,
				)

				Expect(environmentValidator.ValidateCall.Receives.State).To(Equal(incomingState))
				Expect(err).To(MatchError("environment not found"))
			})
		})

		Context("state manipulation", func() {
			Context("when the env id does not exist", func() {
				It("saves state with new certificate name and lb type", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:   "concourse",
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						storage.State{},
					)
					Expect(err).NotTo(HaveOccurred())

					Expect(stateStore.SetCall.CallCount).To(Equal(2))
					state := stateStore.SetCall.Receives[0].State
					Expect(state.LB.Type).To(Equal("concourse"))
				})
			})
		})

		Context("failure cases", func() {
			PDescribeTable("returns an error when an lb already exists",
				func(newLbType, oldLbType string) {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:   "concourse",
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						storage.State{
							LB: storage.LB{
								Type: oldLbType,
							},
						},
					)
					Expect(err).To(MatchError(fmt.Sprintf("bbl already has a %s load balancer attached, please remove the previous load balancer before attaching a new one", oldLbType)))
				},
				Entry("when the previous lb type is concourse", "concourse", "cf"),
				Entry("when the previous lb type is cf", "cf", "concourse"),
			)

			Context("when cert path is invalid", func() {
				It("returns an error", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								CertPath: "/fake/cert/path",
								KeyPath:  keyPath,
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError(ContainSubstring("no such file or directory")))
				})
			})

			Context("when key path is invalid", func() {
				It("returns an error", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								CertPath: certPath,
								KeyPath:  "/fake/key/path",
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError(ContainSubstring("no such file or directory")))
				})
			})

			Context("when chain path is invalid", func() {
				It("returns an error", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								CertPath:  certPath,
								KeyPath:   keyPath,
								ChainPath: "/fake/chain/path",
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError(ContainSubstring("no such file or directory")))
				})
			})

			Context("when terraform manager fails to apply", func() {
				It("saves the bbl state and returns the error", func() {
					terraformManager.ApplyCall.Returns.Error = errors.New("failed to apply")
					terraformManager.ApplyCall.Returns.BBLState = storage.State{
						LB: storage.LB{
							Type: "concourse",
						},
					}

					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError("failed to apply"))

					Expect(stateStore.SetCall.CallCount).To(Equal(2))
					Expect(stateStore.SetCall.Receives[1].State.LB.Type).To(Equal("concourse"))
				})

				Context("when we fail to set the bbl state", func() {
					BeforeEach(func() {
						terraformManager.ApplyCall.Returns.Error = errors.New("failed to apply")
						stateStore.SetCall.Returns = []fakes.SetCallReturn{
							{},
							{errors.New("failed to set bbl state")},
						}
					})

					It("returns the terraform error and the state store set error", func() {
						err := command.Execute(
							commands.CreateLBsConfig{
								AWS: commands.AWSCreateLBsConfig{
									CertPath: certPath,
									KeyPath:  keyPath,
								},
							},
							storage.State{},
						)
						Expect(err).To(MatchError("the following errors occurred:\nfailed to apply,\nfailed to set bbl state"))
					})
				})
			})

			Context("when terraform manager init fails", func() {
				It("returns an error", func() {
					terraformManager.InitCall.Returns.Error = errors.New("clementine")

					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError("clementine"))
				})
			})

			Context("when cloud config manager update fails", func() {
				BeforeEach(func() {
					cloudConfigManager.UpdateCall.Returns.Error = errors.New("failed to update cloud config")
				})

				It("returns an error", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:   "concourse",
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError("failed to update cloud config"))
				})
			})

			Context("when cloud config manager initialize fails", func() {
				BeforeEach(func() {
					cloudConfigManager.InitializeCall.Returns.Error = errors.New("coconut")
				})

				It("returns an error", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								LBType:   "concourse",
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError("coconut"))
				})
			})

			Context("when the state fails to save", func() {
				BeforeEach(func() {
					stateStore.SetCall.Returns = []fakes.SetCallReturn{{errors.New("failed to save state")}}
				})

				It("returns an error", func() {
					err := command.Execute(
						commands.CreateLBsConfig{
							AWS: commands.AWSCreateLBsConfig{
								CertPath: certPath,
								KeyPath:  keyPath,
							},
						},
						storage.State{},
					)
					Expect(err).To(MatchError("failed to save state"))
				})
			})
		})
	})
})
