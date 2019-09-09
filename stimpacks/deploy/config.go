package deploy

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/PremiereGlobal/stim/pkg/utils"
	v2e "github.com/PremiereGlobal/vault-to-envs/pkg/vaulttoenvs"
)

const (
	DEFAULT_CONTAINER_REPO   = "premiereglobal/kube-vault-deploy"
	DEFAULT_CONTAINER_TAG    = "0.3.1"
	DEFAULT_DEPLOY_DIRECTORY = "./"
	DEFAULT_DEPLOY_SCRIPT    = "deploy.sh"
	DEFAULT_CONFIG_FILE      = "./stim.deploy.yaml"
)

// Config is the root structure for the deployment configuration
type Config struct {
	configFilePath string
	Deployment     Deployment     `yaml:"deployment"`
	Container      Container      `yaml:"container"`
	Global         Global         `yaml:"global"`
	Environments   []*Environment `yaml:"environments"`
	environmentMap map[string]int
}

// Deployment describes details about the deployment assets (directories, files, etc)
type Deployment struct {
	Directory         string `yaml:"directory"`
	Script            string `yaml:"script"`
	fullDirectoryPath string
}

// Container describes the container used for Docker deployments
type Container struct {
	Repo string `yaml:"repo"`
	Tag  string `yaml:"tag"`
}

// Global describes global environment specs
type Global struct {
	EnvSpec *EnvSpec `yaml:"envSpec"`
}

// EnvSpec
type EnvSpec struct {
	Kubernetes      Kubernetes        `yaml:"kubernetes"`
	Secrets         []*v2e.SecretItem `yaml:"secrets"`
	EnvironmentVars []*EnvironmentVar `yaml:"env"`
}

// Kubernetes describes the Kubernetes configuration to use
type Kubernetes struct {
	ServiceAccount string `yaml:"serviceAccount"`
	Cluster        string `yaml:"cluster"`
}

// Environment describes a deployment environment (i.e. dev, stage, prod, etc.)
type Environment struct {
	Name        string      `yaml:"name"`
	EnvSpec     *EnvSpec    `yaml:"envSpec"`
	Instances   []*Instance `yaml:"instances"`
	instanceMap map[string]int
}

// Instance describes an instance of a deployment within an environment (i.e. us-west-2 for env prod)
type Instance struct {
	Name    string   `yaml:"name"`
	EnvSpec *EnvSpec `yaml:"envSpec"`
}

// EnvironmentVar describes a shell env var to be injected into the deployment environment
type EnvironmentVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// parseConfig opens the deployment config file and ensures it is valid
func (d *Deploy) parseConfig() {

	d.config = Config{}

	configFile := d.stim.GetConfig("deploy.file")

	if configFile == "" {
		setConfigDefault(&configFile, DEFAULT_CONFIG_FILE)
		d.stim.Debug(fmt.Sprintf("Deployment file not specified, using %s", DEFAULT_CONFIG_FILE))
	}

	_, err := os.Stat(configFile)
	if err != nil && !os.IsExist(err) {
		d.stim.Fatal(errors.New(fmt.Sprintf("No deployment config file exists at: %s", configFile)))
	}

	contentstring, err := ioutil.ReadFile(configFile)
	if err != nil {
		d.stim.Fatal(errors.New(fmt.Sprintf("Deployment config file could not be read: %v", err)))
	}

	if ok, err := utils.IsYaml(contentstring); !ok {
		d.stim.Fatal(errors.New(fmt.Sprintf("Deployment config file (%s) is not valid YAML: %v", configFile, err)))
	}

	err = yaml.Unmarshal([]byte(contentstring), &d.config)
	if err != nil {
		d.stim.Fatal(errors.New(fmt.Sprintf("Error parsing deployment config %v", err)))
	}

	d.config.configFilePath = configFile

	d.processConfig()

}

// processConfig ensures that the deployment config is valid
func (d *Deploy) processConfig() {

	// Set defaults
	setConfigDefault(&d.config.Container.Repo, DEFAULT_CONTAINER_REPO)
	setConfigDefault(&d.config.Container.Tag, DEFAULT_CONTAINER_TAG)
	setConfigDefault(&d.config.Deployment.Directory, DEFAULT_DEPLOY_DIRECTORY)
	setConfigDefault(&d.config.Deployment.Script, DEFAULT_DEPLOY_SCRIPT)

	// Create our global envSpec if it doesn't exist so we don't have to keep checking if it exists
	if d.config.Global.EnvSpec == nil {
		d.config.Global.EnvSpec = &EnvSpec{}
	}

	d.config.environmentMap = make(map[string]int)
	for i, environment := range d.config.Environments {

		// Check to make sure that we don't have multiple environments with the same name
		if _, ok := d.config.environmentMap[environment.Name]; ok {
			d.stim.Fatal(errors.New(fmt.Sprintf("Error parsing config, duplicate environment name `%s` found", environment.Name)))
		}
		d.config.environmentMap[environment.Name] = i

		// Ensure there are instances for this environment
		if len(environment.Instances) <= 0 {
			d.stim.Fatal(errors.New(fmt.Sprintf("No instances found for environment: `%s`", environment.Name)))
		}

		// Create our environment envSpec if it doesn't exist so we don't have to keep checking if it exists
		if environment.EnvSpec == nil {
			environment.EnvSpec = &EnvSpec{}
		}

		environment.instanceMap = make(map[string]int)
		for j, instance := range environment.Instances {

			// Check to make sure that we don't have multiple instances with the same name
			if _, ok := environment.instanceMap[instance.Name]; ok {
				d.stim.Fatal(errors.New(fmt.Sprintf("Error parsing config, duplicate instance name `%s` for environment '%s'", instance.Name, environment.Name)))
			}
			environment.instanceMap[instance.Name] = j

			// Ensure the instance name does not conflict with the ALL option name.  This is a reserved name for designating a deployment to all instances in an environment via the manual prompt list
			if strings.ToLower(instance.Name) == strings.ToLower(ALL_OPTION_TEXT) {
				d.stim.Fatal(errors.New(fmt.Sprintf("Deployment config cannot have an instance named '%s'. It is a reserved name.", instance.Name)))
			}

			// Create our instance envSpec if it doesn't exist so we don't have to keep checking if it exists
			if instance.EnvSpec == nil {
				instance.EnvSpec = &EnvSpec{}
			}

			// Merge all of the secrets and environment variables
			// Instance-level specs take precedence, followed by environment-level then global-level
			if instance.EnvSpec.Kubernetes.ServiceAccount == "" {
				if environment.EnvSpec.Kubernetes.ServiceAccount != "" {
					instance.EnvSpec.Kubernetes.ServiceAccount = environment.EnvSpec.Kubernetes.ServiceAccount
				} else {
					instance.EnvSpec.Kubernetes.ServiceAccount = d.config.Global.EnvSpec.Kubernetes.ServiceAccount
				}

			}
			if instance.EnvSpec.Kubernetes.Cluster == "" {
				if environment.EnvSpec.Kubernetes.Cluster != "" {
					instance.EnvSpec.Kubernetes.Cluster = environment.EnvSpec.Kubernetes.Cluster
				} else {
					instance.EnvSpec.Kubernetes.Cluster = d.config.Global.EnvSpec.Kubernetes.Cluster
				}

			}

			instance.EnvSpec.EnvironmentVars = mergeEnvVars(instance.EnvSpec.EnvironmentVars, environment.EnvSpec.EnvironmentVars, d.config.Global.EnvSpec.EnvironmentVars)
			instance.EnvSpec.Secrets = mergeSecrets(instance.EnvSpec.Secrets, environment.EnvSpec.Secrets, d.config.Global.EnvSpec.Secrets)

			// Ensure a Kubernetes cluster is set
			if instance.EnvSpec.Kubernetes.Cluster == "" {
				d.stim.Fatal(errors.New(fmt.Sprintf("Kubernetes cluster is not set for instance '%s' in environment '%s'", instance.Name, environment.Name)))
			}

			// Get Vault details
			vault := d.stim.Vault()
			vaultToken, err := vault.GetToken()
			if err != nil {
				panic(err)
			}

			vaultAddress, err := vault.GetAddress()
			if err != nil {
				panic(err)
			}

			// Generate stim env vars
			stimEnvs := []*EnvironmentVar{}

			stimEnvs = append(stimEnvs, []*EnvironmentVar{
				&EnvironmentVar{Name: "VAULT_ADDR", Value: vaultAddress},
				&EnvironmentVar{Name: "VAULT_TOKEN", Value: vaultToken},
				&EnvironmentVar{Name: "DEPLOY_ENVIRONMENT", Value: environment.Name},
				&EnvironmentVar{Name: "DEPLOY_INSTANCE", Value: instance.Name},
				&EnvironmentVar{Name: "DEPLOY_CLUSTER", Value: instance.EnvSpec.Kubernetes.Cluster},
			}...)

			// Generate the Kube config secret
			var stimSecrets []*v2e.SecretItem
			if instance.EnvSpec.Kubernetes.ServiceAccount != "" {
				secretMap := make(map[string]string)
				secretMap["CLUSTER_SERVER"] = "cluster-server"
				secretMap["CLUSTER_CA"] = "cluster-ca"
				secretMap["USER_TOKEN"] = "user-token"
				stimSecrets = append(stimSecrets, &v2e.SecretItem{
					SecretPath: fmt.Sprintf("secret/kubernetes/%s/%s/kube-config", instance.EnvSpec.Kubernetes.Cluster, instance.EnvSpec.Kubernetes.ServiceAccount),
					SecretMaps: secretMap,
				})
			} else {
				d.stim.Fatal(errors.New("Kubernetes service account required but not provided"))
			}

			// Add stim envs/secrets and ensure no reserved env vars have been set
			// d.addStimEnvs(e, inst)
			d.finalizeEnv(instance, stimEnvs, stimSecrets)
		}
	}

	// Determine the full directory path
	d.config.Deployment.fullDirectoryPath = filepath.Join(filepath.Dir(d.config.configFilePath), d.config.Deployment.Directory)

}

// Generate the list of reserved env var names
func (d *Deploy) finalizeEnv(instance *Instance, stimEnvs []*EnvironmentVar, stimSecrets []*v2e.SecretItem) {

	// Generate the list of reserved env var names (additionally SECRET_CONFIG as we'll add that one at the end)
	reservedVarNames := []string{"SECRET_CONFIG"}
	for _, s := range stimEnvs {
		reservedVarNames = append(reservedVarNames, s.Name)
	}
	for _, s := range stimSecrets {
		for m, _ := range s.SecretMaps {
			reservedVarNames = append(reservedVarNames, m)
		}
	}

	// Exit if any user-provided environment vars conflict with reserved ones
	for _, e := range instance.EnvSpec.EnvironmentVars {
		if utils.Contains(reservedVarNames, e.Name) {
			d.stim.Fatal(errors.New(fmt.Sprintf("Reserved environment variable name '%s' found in config", e.Name)))
		}
	}
	for _, s := range instance.EnvSpec.Secrets {
		for m, _ := range s.SecretMaps {
			if utils.Contains(reservedVarNames, m) {
				d.stim.Fatal(errors.New(fmt.Sprintf("Reserved environment variable name '%s' found in config", m)))
			}
		}
	}

	// Combine our secrets
	instance.EnvSpec.Secrets = append(instance.EnvSpec.Secrets, stimSecrets...)

	// Create the secret config
	secretConfig, err := d.makeSecretConfig(instance)
	if err != nil {
		panic(err)
	}
	stimEnvs = append(stimEnvs, &EnvironmentVar{Name: "SECRET_CONFIG", Value: secretConfig})

	// Combine our env vars
	instance.EnvSpec.EnvironmentVars = append(instance.EnvSpec.EnvironmentVars, stimEnvs...)

}

// mergeEnvVars is used to merge environment variable configuration at the various levels it can be set at
func mergeEnvVars(instance []*EnvironmentVar, environment []*EnvironmentVar, global []*EnvironmentVar) []*EnvironmentVar {

	result := instance

	// Add environment envVars (if they don't already exist)
	for _, e := range environment {
		exists := false
		for _, inst := range instance {
			if inst.Name == e.Name {
				exists = true
			}
		}

		// Add the item if it doesn't exist
		if !exists {
			result = append(result, e)
		}
	}

	// Add global envVars (if they don't already exist)
	for _, g := range global {
		exists := false
		for _, inst := range instance {
			if inst.Name == g.Name {
				exists = true
			}
		}

		// Add the item if it doesn't exist
		if !exists {
			result = append(result, g)
		}
	}

	return result
}

// mergeSecrets is used to merge secret configs at the various levels they can be set at
func mergeSecrets(instance []*v2e.SecretItem, environment []*v2e.SecretItem, global []*v2e.SecretItem) []*v2e.SecretItem {

	result := global

	// Add environment envVars
	for _, e := range environment {
		result = append(result, e)
	}

	// Add global envVars to instance (if they don't already exist)
	for _, inst := range instance {
		result = append(result, inst)
	}

	return result
}

// setConfigDefault is used to set a default value (if it doesn't exist)
func setConfigDefault(value *string, def string) {
	if len(*value) == 0 {
		*value = def
	}
}
