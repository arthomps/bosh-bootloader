package terraform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cloudfoundry/bosh-bootloader/storage"
)

var writeFile func(file string, data []byte, perm os.FileMode) error = ioutil.WriteFile
var readFile func(filename string) ([]byte, error) = ioutil.ReadFile

type Executor struct {
	cmd        terraformCmd
	stateStore stateStore
	debug      bool
}

type ImportInput struct {
	TerraformAddr string
	AWSResourceID string
	TFState       string
	Creds         storage.AWS
}

type tfOutput struct {
	Sensitive bool
	Type      string
	Value     interface{}
}

type terraformCmd interface {
	Run(stdout io.Writer, workingDirectory string, args []string, debug bool) error
}

type stateStore interface {
	GetTerraformDir() (string, error)
	GetVarsDir() (string, error)
}

func NewExecutor(cmd terraformCmd, stateStore stateStore, debug bool) Executor {
	return Executor{
		cmd:        cmd,
		stateStore: stateStore,
		debug:      debug,
	}
}

func (e Executor) Init(template, prevTFState string) error {
	terraformDir, err := e.stateStore.GetTerraformDir()
	if err != nil {
		return fmt.Errorf("Get terraform dir: %s", err)
	}

	err = writeFile(filepath.Join(terraformDir, "template.tf"), []byte(template), os.ModePerm)
	if err != nil {
		return fmt.Errorf("Write terraform template: %s", err)
	}

	varsDir, err := e.stateStore.GetVarsDir()
	if err != nil {
		return fmt.Errorf("Get vars dir: %s", err)
	}

	tfStatePath := filepath.Join(varsDir, "terraform.tfstate")
	if prevTFState != "" {
		// TODO: this should live in a central state migration package
		// to migrate state from previous bbl versions
		err = writeFile(tfStatePath, []byte(prevTFState), os.ModePerm)
		if err != nil {
			return fmt.Errorf("Write previous terraform state: %s", err)
		}
	}

	err = os.MkdirAll(filepath.Join(terraformDir, ".terraform"), os.ModePerm)
	if err != nil {
		return fmt.Errorf("Create .terraform directory: %s", err)
	}

	err = writeFile(filepath.Join(terraformDir, ".terraform", ".gitignore"), []byte("*\n"), os.ModePerm)
	if err != nil {
		return fmt.Errorf("Write .gitignore for terraform binaries: %s", err)
	}

	err = e.cmd.Run(os.Stdout, terraformDir, []string{"init"}, e.debug)
	if err != nil {
		return fmt.Errorf("Run terraform init: %s", err)
	}

	return nil
}

func (e Executor) Apply(input map[string]string) (string, error) {
	varsDir, err := e.stateStore.GetVarsDir()
	if err != nil {
		return "", fmt.Errorf("Get vars dir: %s", err)
	}
	tfStatePath := filepath.Join(varsDir, "terraform.tfstate")

	terraformDir, err := e.stateStore.GetTerraformDir()
	if err != nil {
		return "", fmt.Errorf("Get terraform dir: %s", err)
	}
	relativeStatePath, err := filepath.Rel(terraformDir, tfStatePath)
	if err != nil {
		return "", fmt.Errorf("Get relative terraform state path: %s", err) //not tested
	}

	args := []string{
		"apply",
		"-state", relativeStatePath,
	}
	for name, value := range input {
		tfVar := []string{"-var", fmt.Sprintf("%s=%s", name, value)}
		args = append(args, tfVar...)
	}

	err = e.cmd.Run(os.Stdout, terraformDir, args, e.debug)
	if err != nil {
		return "", NewExecutorError(tfStatePath, err, e.debug)
	}

	tfState, err := readFile(tfStatePath)
	if err != nil {
		return "", fmt.Errorf("Read terraform state: %s", err)
	}

	return string(tfState), nil
}

func (e Executor) Destroy(input map[string]string) (string, error) {
	terraformDir, err := e.stateStore.GetTerraformDir()
	if err != nil {
		return "", fmt.Errorf("Get terraform dir: %s", err)
	}

	varsDir, err := e.stateStore.GetVarsDir()
	if err != nil {
		return "", fmt.Errorf("Get vars dir: %s", err)
	}

	tfStatePath := filepath.Join(varsDir, "terraform.tfstate")

	relativeStatePath, err := filepath.Rel(terraformDir, tfStatePath)
	if err != nil {
		return "", fmt.Errorf("Get relative terraform state path: %s", err) //not tested
	}

	args := []string{
		"destroy",
		"-force",
		"-state", relativeStatePath,
	}
	for name, value := range input {
		tfVar := []string{"-var", fmt.Sprintf("%s=%s", name, value)}
		args = append(args, tfVar...)
	}

	err = e.cmd.Run(os.Stdout, terraformDir, args, e.debug)
	if err != nil {
		return "", NewExecutorError(tfStatePath, err, e.debug)
	}

	tfState, err := readFile(tfStatePath)
	if err != nil {
		return "", fmt.Errorf("Read terraform state: %s", err)
	}

	return string(tfState), nil
}

func (e Executor) Import(input ImportInput) (string, error) {
	terraformDir, err := e.stateStore.GetTerraformDir()
	if err != nil {
		return "", err
	}

	resourceType := strings.Split(input.TerraformAddr, ".")[0]
	resourceName := strings.Split(input.TerraformAddr, ".")[1]
	resourceName = strings.Split(resourceName, "[")[0]

	template := fmt.Sprintf(`
provider "aws" {
	region     = %q
	access_key = %q
	secret_key = %q
}

resource %q %q {
}`, input.Creds.Region, input.Creds.AccessKeyID, input.Creds.SecretAccessKey, resourceType, resourceName)

	err = writeFile(filepath.Join(terraformDir, "template.tf"), []byte(template), os.ModePerm)
	if err != nil {
		return "", err
	}

	varsDir, err := e.stateStore.GetVarsDir()
	if err != nil {
		return "", err
	}

	tfStatePath := filepath.Join(varsDir, "terraform.tfstate")

	err = writeFile(tfStatePath, []byte(input.TFState), os.ModePerm)
	if err != nil {
		return "", err
	}

	err = e.cmd.Run(os.Stdout, terraformDir, []string{"init"}, e.debug)
	if err != nil {
		return "", err
	}

	relativeStatePath, err := filepath.Rel(terraformDir, tfStatePath)
	if err != nil {
		return "", fmt.Errorf("Get relative terraform state path: %s", err) //not tested
	}

	err = e.cmd.Run(os.Stdout, terraformDir, []string{"import", input.TerraformAddr, input.AWSResourceID, "-state", relativeStatePath}, e.debug)
	if err != nil {
		return "", fmt.Errorf("failed to import: %s", err)
	}

	tfStateContents, err := readFile(tfStatePath)
	if err != nil {
		return "", err
	}

	return string(tfStateContents), nil
}

func (e Executor) Version() (string, error) {
	buffer := bytes.NewBuffer([]byte{})
	err := e.cmd.Run(buffer, "/tmp", []string{"version"}, true)
	if err != nil {
		return "", err
	}
	versionOutput := buffer.String()
	regex := regexp.MustCompile(`\d+.\d+.\d+`)

	version := regex.FindString(versionOutput)
	if version == "" {
		return "", errors.New("Terraform version could not be parsed")
	}

	return version, nil
}

func (e Executor) Output(tfState, outputName string) (string, error) {
	terraformDir, err := e.stateStore.GetTerraformDir()
	if err != nil {
		return "", fmt.Errorf("Get terraform dir: %s", err)
	}

	err = writeFile(filepath.Join(terraformDir, "terraform.tfstate"), []byte(tfState), os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("Write terraform state to terraform.tfstate in terraform dir: %s", err)
	}

	varsDir, err := e.stateStore.GetVarsDir()
	if err != nil {
		return "", fmt.Errorf("Get vars dir: %s", err)
	}

	err = e.cmd.Run(os.Stdout, terraformDir, []string{"init"}, e.debug)
	if err != nil {
		return "", fmt.Errorf("Run terraform init in terraform dir: %s", err)
	}

	args := []string{"output", outputName, "-state", filepath.Join(varsDir, "terraform.tfstate")}
	buffer := bytes.NewBuffer([]byte{})
	err = e.cmd.Run(buffer, terraformDir, args, true)
	if err != nil {
		return "", fmt.Errorf("Run terraform output -state: %s", err)
	}

	return strings.TrimSuffix(buffer.String(), "\n"), nil
}

func (e Executor) Outputs(tfState string) (map[string]interface{}, error) {
	varsDir, err := e.stateStore.GetVarsDir()
	if err != nil {
		return map[string]interface{}{}, fmt.Errorf("Get vars dir: %s", err)
	}

	err = writeFile(filepath.Join(varsDir, "terraform.tfstate"), []byte(tfState), os.ModePerm)
	if err != nil {
		return map[string]interface{}{}, fmt.Errorf("Write terraform state to terraform.tfstate: %s", err)
	}

	err = e.cmd.Run(os.Stdout, varsDir, []string{"init"}, false)
	if err != nil {
		return map[string]interface{}{}, fmt.Errorf("Run terraform init in vars dir: %s", err)
	}

	buffer := bytes.NewBuffer([]byte{})
	err = e.cmd.Run(buffer, varsDir, []string{"output", "--json"}, true)
	if err != nil {
		return map[string]interface{}{}, fmt.Errorf("Run terraform output --json in vars dir: %s", err)
	}

	tfOutputs := map[string]tfOutput{}
	err = json.Unmarshal(buffer.Bytes(), &tfOutputs)
	if err != nil {
		return map[string]interface{}{}, fmt.Errorf("Unmarshal terraform output: %s", err)
	}

	outputs := map[string]interface{}{}
	for tfKey, tfValue := range tfOutputs {
		outputs[tfKey] = tfValue.Value
	}

	return outputs, nil
}
