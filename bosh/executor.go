package bosh

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Executor struct {
	command       command
	readFile      func(string) ([]byte, error)
	unmarshalJSON func([]byte, interface{}) error
	marshalJSON   func(interface{}) ([]byte, error)
	writeFile     func(string, []byte, os.FileMode) error
}

type InterpolateInput struct {
	DeploymentDir string
	StateDir      string
	VarsDir       string
	IAAS          string
	BOSHState     map[string]interface{}
	Variables     string
	OpsFile       string
}

type CreateEnvInput struct {
	StateDir       string
	VarsDir        string
	Deployment     string
	DeploymentVars string
}

type DeleteEnvInput struct {
	StateDir   string
	VarsDir    string
	Deployment string
}

type command interface {
	GetBOSHPath() (string, error)
	Run(stdout io.Writer, workingDirectory string, args []string) error
}

type setupFile struct {
	path     string
	contents []byte
}

const VERSION_DEV_BUILD = "[DEV BUILD]"

var (
	jumpboxDeploymentRepo = "vendor/github.com/cppforlife/jumpbox-deployment"
	boshDeploymentRepo    = "vendor/github.com/cloudfoundry/bosh-deployment"
)

func NewExecutor(cmd command, readFile func(string) ([]byte, error),
	unmarshalJSON func([]byte, interface{}) error,
	marshalJSON func(interface{}) ([]byte, error),
	writeFile func(string, []byte, os.FileMode) error) Executor {
	return Executor{
		command:       cmd,
		readFile:      readFile,
		unmarshalJSON: unmarshalJSON,
		marshalJSON:   marshalJSON,
		writeFile:     writeFile,
	}
}

func (e Executor) getJumpboxSetupFiles(input InterpolateInput) []setupFile {
	return []setupFile{
		setupFile{
			path:     filepath.Join(input.DeploymentDir, "jumpbox.yml"),
			contents: MustAsset(filepath.Join(jumpboxDeploymentRepo, "jumpbox.yml")),
		},
		setupFile{
			path:     filepath.Join(input.DeploymentDir, "cpi.yml"),
			contents: MustAsset(filepath.Join(jumpboxDeploymentRepo, input.IAAS, "cpi.yml")),
		},
	}
}

func (e Executor) getCreateEnvScripts(input InterpolateInput, deployment string) []setupFile {
	return []setupFile{
		setupFile{path: filepath.Join(input.StateDir, fmt.Sprintf("create-%s.sh", deployment))},
		setupFile{path: filepath.Join(input.StateDir, fmt.Sprintf("delete-%s.sh", deployment))},
	}
}

func (e Executor) allFilesExist(files []setupFile) bool {
	for _, f := range files {
		_, err := os.Stat(f.path)
		if err != nil {
			return false
		}
	}
	return true
}

func (e Executor) IsJumpboxInitialized(input InterpolateInput) bool {
	setupFiles := e.getJumpboxSetupFiles(input)
	createEnvScripts := e.getCreateEnvScripts(input, "jumpbox")
	return e.allFilesExist(setupFiles) && e.allFilesExist(createEnvScripts)
}

func (e Executor) JumpboxCreateEnvArgs(input InterpolateInput) error {
	setupFiles := e.getJumpboxSetupFiles(input)

	varsStoreFile := setupFile{
		path:     filepath.Join(input.VarsDir, "jumpbox-variables.yml"),
		contents: []byte(input.Variables),
	}

	setupFiles = append(setupFiles, varsStoreFile)

	for _, f := range setupFiles {
		err := e.writeFile(f.path, f.contents, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Jumpbox write setup file: %s", err) //not tested
		}
	}

	sharedArgs := []string{
		"--vars-store", varsStoreFile.path,
		"--vars-file", filepath.Join(input.VarsDir, "jumpbox-deployment-vars.yml"),
		"-o", setupFiles[1].path,
	}

	jumpboxState := filepath.Join(input.VarsDir, "jumpbox-state.json")
	if input.BOSHState != nil {
		stateJSON, err := e.marshalJSON(input.BOSHState)
		if err != nil {
			return fmt.Errorf("Jumpbox marshal state json: %s", err) //not tested
		}

		err = e.writeFile(jumpboxState, stateJSON, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Jumpbox write state json: %s", err) //not tested
		}
	}

	boshArgs := append([]string{
		setupFiles[0].path,
		"--state", jumpboxState,
	}, sharedArgs...)

	boshPath, err := e.command.GetBOSHPath()
	if err != nil {
		return fmt.Errorf("Jumpbox get BOSH path: %s", err) //not tested
	}

	createEnvCmd := []byte(formatScript(boshPath, input.StateDir, "create-env", boshArgs))
	createJumpboxScript := filepath.Join(input.StateDir, "create-jumpbox.sh")
	err = e.writeFile(createJumpboxScript, createEnvCmd, os.ModePerm)
	if err != nil {
		return err
	}

	deleteEnvCmd := []byte(formatScript(boshPath, input.StateDir, "delete-env", boshArgs))
	deleteJumpboxScript := filepath.Join(input.StateDir, "delete-jumpbox.sh")
	err = e.writeFile(deleteJumpboxScript, deleteEnvCmd, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func (e Executor) getDirectorSetupFiles(input InterpolateInput) []setupFile {
	files := []setupFile{
		setupFile{
			path:     filepath.Join(input.DeploymentDir, "bosh.yml"),
			contents: MustAsset(filepath.Join(boshDeploymentRepo, "bosh.yml")),
		},
	}

	assetNames := AssetNames()
	for _, asset := range assetNames {
		if strings.Contains(asset, boshDeploymentRepo) {
			files = append(files, setupFile{
				name:     strings.TrimPrefix(asset, boshDeploymentRepo),
				path:     filepath.Join(input.DeploymentDir, strings.TrimPrefix(asset, boshDeploymentRepo)),
				contents: MustAsset(asset),
			})
		}
	}

	return files
}

func (e Executor) getDirectorOpsFiles(input InterpolateInput) []setupFile {
	files := []setupFile{
		setupFile{
			path:     filepath.Join(input.DeploymentDir, "cpi.yml"),
			contents: MustAsset(filepath.Join(boshDeploymentRepo, input.IAAS, "cpi.yml")),
		},
		setupFile{
			path:     filepath.Join(input.DeploymentDir, "jumpbox-user.yml"),
			contents: MustAsset(filepath.Join(boshDeploymentRepo, "jumpbox-user.yml")),
		},
		setupFile{
			path:     filepath.Join(input.DeploymentDir, "uaa.yml"),
			contents: MustAsset(filepath.Join(boshDeploymentRepo, "uaa.yml")),
		},
		setupFile{
			path:     filepath.Join(input.DeploymentDir, "credhub.yml"),
			contents: MustAsset(filepath.Join(boshDeploymentRepo, "credhub.yml")),
		},
	}
	if input.IAAS == "gcp" {
		files = append(files, setupFile{
			path:     filepath.Join(input.DeploymentDir, "gcp-bosh-director-ephemeral-ip-ops.yml"),
			contents: []byte(GCPBoshDirectorEphemeralIPOps),
		})
	}
	if input.IAAS == "aws" {
		files = append(files, setupFile{
			path:     filepath.Join(input.DeploymentDir, "aws-bosh-director-ephemeral-ip-ops.yml"),
			contents: []byte(AWSBoshDirectorEphemeralIPOps),
		})
		files = append(files, setupFile{
			path:     filepath.Join(input.DeploymentDir, "iam-instance-profile.yml"),
			contents: MustAsset(filepath.Join(boshDeploymentRepo, input.IAAS, "iam-instance-profile.yml")),
		})
		files = append(files, setupFile{
			path:     filepath.Join(input.DeploymentDir, "aws-bosh-director-encrypt-disk-ops.yml"),
			contents: []byte(AWSEncryptDiskOps),
		})
	}
	return files
}

func (e Executor) IsDirectorInitialized(input InterpolateInput) bool {
	setupFiles := e.getDirectorSetupFiles(input)
	opsFiles := e.getDirectorOpsFiles(input)
	createEnvScripts := e.getCreateEnvScripts(input, "director")
	return e.allFilesExist(setupFiles) && e.allFilesExist(opsFiles) && e.allFilesExist(createEnvScripts)
}

func (e Executor) DirectorCreateEnvArgs(input InterpolateInput) error {
	setupFiles := e.getDirectorSetupFiles(input)
	varsStoreFile := setupFile{
		path:     filepath.Join(input.VarsDir, "director-variables.yml"),
		contents: []byte(input.Variables),
	}
	userOpsFile := setupFile{
		path:     filepath.Join(input.VarsDir, "user-ops-file.yml"),
		contents: []byte(input.OpsFile),
	}
	setupFiles = append(setupFiles, varsStoreFile, userOpsFile)
	opsFiles := e.getDirectorOpsFiles(input)

	for _, f := range append(setupFiles, opsFiles...) {
		if f.name != "" {
			os.MkdirAll(f.name)
		}
		if err := e.writeFile(f.path, f.contents, os.ModePerm); err != nil {
			return fmt.Errorf("Director write setup file: %s", err) //not tested
		}
	}

	sharedArgs := []string{
		"--vars-store", varsStoreFile.path,
		"--vars-file", filepath.Join(input.VarsDir, "director-deployment-vars.yml"),
	}

	for _, f := range opsFiles {
		sharedArgs = append(sharedArgs, "-o", f.path)
	}

	if input.OpsFile != "" {
		sharedArgs = append(sharedArgs, "-o", filepath.Join(input.VarsDir, "user-ops-file.yml"))
	}

	boshState := filepath.Join(input.VarsDir, "bosh-state.json")
	if input.BOSHState != nil {
		stateJSON, err := e.marshalJSON(input.BOSHState)
		if err != nil {
			return fmt.Errorf("marshal JSON: %s", err) //not tested
		}

		err = e.writeFile(boshState, stateJSON, os.ModePerm)
		if err != nil {
			return fmt.Errorf("write file: %s", err) //not tested
		}
	}

	boshPath, err := e.command.GetBOSHPath()
	if err != nil {
		return fmt.Errorf("Director get BOSH path: %s", err) //not tested
	}

	boshArgs := append([]string{
		setupFiles[0].path,
		"--state", boshState,
	}, sharedArgs...)

	createEnvCmd := []byte(formatScript(boshPath, input.StateDir, "create-env", boshArgs))
	err = e.writeFile(filepath.Join(input.StateDir, "create-director.sh"), createEnvCmd, os.ModePerm)
	if err != nil {
		return err
	}

	deleteEnvCmd := []byte(formatScript(boshPath, input.StateDir, "delete-env", boshArgs))
	err = e.writeFile(filepath.Join(input.StateDir, "delete-director.sh"), deleteEnvCmd, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func formatScript(boshPath, stateDir, command string, args []string) string {
	script := fmt.Sprintf("#!/bin/sh\n%s %s \\\n", boshPath, command)
	for _, arg := range args {
		if arg[0] == '-' {
			script = fmt.Sprintf("%s  %s", script, arg)
		} else {
			script = fmt.Sprintf("%s  %s \\\n", script, arg)
		}
	}
	script = strings.Replace(script, stateDir, "${BBL_STATE_DIR}", -1)
	return fmt.Sprintf("%s\n", script[:len(script)-2])
}

func (e Executor) CreateEnv(createEnvInput CreateEnvInput) (string, error) {
	os.Setenv("BBL_STATE_DIR", createEnvInput.StateDir)
	createEnvScript := filepath.Join(createEnvInput.StateDir, fmt.Sprintf("create-%s.sh", createEnvInput.Deployment))

	varsFilePath := filepath.Join(createEnvInput.VarsDir, fmt.Sprintf("%s-deployment-vars.yml", createEnvInput.Deployment))
	err := e.writeFile(varsFilePath, []byte(createEnvInput.DeploymentVars), os.ModePerm)
	if err != nil {
		return "", fmt.Errorf("Write vars file: %s", err) // not tested
	}

	cmd := exec.Command(createEnvScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return "", fmt.Errorf("Run bosh create-env: %s", err)
	}

	varsStoreFileName := fmt.Sprintf("%s-variables.yml", createEnvInput.Deployment)
	varsStoreContents, err := e.readFile(filepath.Join(createEnvInput.VarsDir, varsStoreFileName))
	if err != nil {
		return "", fmt.Errorf("Reading vars file for %s deployment: %s", createEnvInput.Deployment, err) // not tested
	}

	return string(varsStoreContents), nil
}

func (e Executor) DeleteEnv(deleteEnvInput DeleteEnvInput) error {
	os.Setenv("BBL_STATE_DIR", deleteEnvInput.StateDir)
	deleteEnvScript := filepath.Join(deleteEnvInput.StateDir, fmt.Sprintf("delete-%s.sh", deleteEnvInput.Deployment))

	cmd := exec.Command(deleteEnvScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("Run bosh delete-env: %s", err)
	}

	return nil
}

func (e Executor) Version() (string, error) {
	args := []string{"-v"}
	buffer := bytes.NewBuffer([]byte{})
	err := e.command.Run(buffer, "", args)
	if err != nil {
		return "", err
	}

	versionOutput := buffer.String()
	regex := regexp.MustCompile(`\d+.\d+.\d+`)

	version := regex.FindString(versionOutput)
	if version == "" {
		return "", NewBOSHVersionError(errors.New("BOSH version could not be parsed"))
	}

	return version, nil
}
