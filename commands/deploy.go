package commands

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"regexp"
	"strings"

	"gopkg.in/urfave/cli.v1"

	"github.com/EngineerBetter/control-tower/bosh"
	"github.com/EngineerBetter/control-tower/certs"
	"github.com/EngineerBetter/control-tower/commands/deploy"
	"github.com/EngineerBetter/control-tower/concourse"
	"github.com/EngineerBetter/control-tower/config"
	"github.com/EngineerBetter/control-tower/fly"
	"github.com/EngineerBetter/control-tower/iaas"
	"github.com/EngineerBetter/control-tower/resource"
	"github.com/EngineerBetter/control-tower/terraform"
	"github.com/EngineerBetter/control-tower/util"
)

const maxAllowedNameLength = 11

var initialDeployArgs deploy.Args

var deployFlags = []cli.Flag{
	cli.StringFlag{
		Name:        "region",
		Usage:       "(optional) AWS region",
		EnvVar:      "AWS_REGION",
		Destination: &initialDeployArgs.Region,
	},
	cli.StringFlag{
		Name:        "domain",
		Usage:       "(optional) Domain to use as endpoint for Concourse web interface (eg: ci.myproject.com)",
		EnvVar:      "DOMAIN",
		Destination: &initialDeployArgs.Domain,
	},
	cli.StringFlag{
		Name:        "tls-cert",
		Usage:       "(optional) TLS cert to use with Concourse endpoint",
		EnvVar:      "TLS_CERT",
		Destination: &initialDeployArgs.TLSCert,
	},
	cli.StringFlag{
		Name:        "tls-key",
		Usage:       "(optional) TLS private key to use with Concourse endpoint",
		EnvVar:      "TLS_KEY",
		Destination: &initialDeployArgs.TLSKey,
	},
	cli.IntFlag{
		Name:        "workers",
		Usage:       "(optional) Number of Concourse worker instances to deploy",
		EnvVar:      "WORKERS",
		Value:       1,
		Destination: &initialDeployArgs.WorkerCount,
	},
	cli.StringFlag{
		Name:        "worker-size",
		Usage:       "(optional) Size of Concourse workers. Can be medium, large, xlarge, 2xlarge, 4xlarge, 12xlarge or 24xlarge",
		EnvVar:      "WORKER_SIZE",
		Value:       "xlarge",
		Destination: &initialDeployArgs.WorkerSize,
	},
	cli.StringFlag{
		Name:        "worker-type",
		Usage:       "(optional) Specify a worker type for aws (m5, m5a, or m4)",
		EnvVar:      "WORKER_TYPE",
		Value:       "m4",
		Destination: &initialDeployArgs.WorkerType,
	},
	cli.StringFlag{
		Name:        "web-size",
		Usage:       "(optional) Size of Concourse web node. Can be small, medium, large, xlarge, 2xlarge",
		EnvVar:      "WEB_SIZE",
		Value:       "small",
		Destination: &initialDeployArgs.WebSize,
	},
	cli.StringFlag{
		Name:        "iaas",
		Usage:       "(required) IAAS, can be AWS or GCP",
		EnvVar:      "IAAS",
		Destination: &initialDeployArgs.IAAS,
	},
	cli.BoolFlag{
		Name:        "self-update",
		Usage:       "(optional) Causes Control-Tower to exit as soon as the BOSH deployment starts. May only be used when upgrading an existing deployment",
		EnvVar:      "SELF_UPDATE",
		Hidden:      true,
		Destination: &initialDeployArgs.SelfUpdate,
	},
	cli.BoolFlag{
		Name:        "enable-global-resources",
		Usage:       "(optional) Enables Concourse global resources. Can be true/false (default: false)",
		EnvVar:      "ENABLE_GLOBAL_RESOURCES",
		Destination: &initialDeployArgs.EnableGlobalResources,
	},
	cli.BoolFlag{
		Name:        "enable-pipeline-instances",
		Usage:       "(optional) Enables Concourse pipeline instances. Can be true/false (default: false)",
		EnvVar:      "ENABLE_PIPELINE_INSTANCES",
		Destination: &initialDeployArgs.EnablePipelineInstances,
	},
	cli.StringFlag{
		Name:        "influxdb-retention-period",
		Usage:       "(optional) Sets influxdb retention period. (default: 28d)",
		EnvVar:      "INFLUXDB_RETENTION_PERIOD",
		Destination: &initialDeployArgs.InfluxDbRetention,
	},
	cli.StringFlag{
		Name:        "db-size",
		Usage:       "(optional) Size of Concourse RDS instance. Can be small, medium, large, xlarge, 2xlarge, or 4xlarge",
		EnvVar:      "DB_SIZE",
		Value:       "small",
		Destination: &initialDeployArgs.DBSize,
	},
	cli.BoolTFlag{
		Name:        "spot",
		Usage:       "(optional) Use spot instances for workers. Can be true/false (default: true)",
		EnvVar:      "SPOT",
		Destination: &initialDeployArgs.Spot,
	},
	cli.BoolTFlag{
		Name:        "preemptible",
		Usage:       "(optional) Use preemptible instances for workers. Can be true/false (default: true)",
		EnvVar:      "PREEMPTIBLE",
		Destination: &initialDeployArgs.Spot,
	},
	cli.StringFlag{
		Name:        "allow-ips",
		Usage:       "(optional) Comma separated list of IP addresses or CIDR ranges to allow access to. Not applied to future manual deploys unless this flag is provided again",
		EnvVar:      "ALLOW_IPS",
		Value:       "0.0.0.0/0",
		Destination: &initialDeployArgs.AllowIPs,
	},
	cli.StringFlag{
		Name:        "bitbucket-auth-client-id",
		Usage:       "(optional) Client ID for a bitbucket OAuth application - Used for Bitbucket Auth",
		EnvVar:      "BITBUCKET_AUTH_CLIENT_ID",
		Destination: &initialDeployArgs.BitbucketAuthClientID,
	},
	cli.StringFlag{
		Name:        "bitbucket-auth-client-secret",
		Usage:       "(optional) Client Secret for a bitbucket OAuth application - Used for Bitbucket Auth",
		EnvVar:      "BITBUCKET_AUTH_CLIENT_SECRET",
		Destination: &initialDeployArgs.BitbucketAuthClientSecret,
	},
	cli.StringFlag{
		Name:        "github-auth-client-id",
		Usage:       "(optional) Client ID for a github OAuth application - Used for Github Auth",
		EnvVar:      "GITHUB_AUTH_CLIENT_ID",
		Destination: &initialDeployArgs.GithubAuthClientID,
	},
	cli.StringFlag{
		Name:        "github-auth-client-secret",
		Usage:       "(optional) Client Secret for a github OAuth application - Used for Github Auth",
		EnvVar:      "GITHUB_AUTH_CLIENT_SECRET",
		Destination: &initialDeployArgs.GithubAuthClientSecret,
	},
	cli.StringFlag{
		Name:        "microsoft-auth-client-id",
		Usage:       "(optional) Client ID for a microsoft OAuth application - Used for Microsoft Auth",
		EnvVar:      "MICROSOFT_AUTH_CLIENT_ID",
		Destination: &initialDeployArgs.MicrosoftAuthClientID,
	},
	cli.StringFlag{
		Name:        "microsoft-auth-client-secret",
		Usage:       "(optional) Client Secret for a microsoft OAuth application - Used for Microsoft Auth",
		EnvVar:      "MICROSOFT_AUTH_CLIENT_SECRET",
		Destination: &initialDeployArgs.MicrosoftAuthClientSecret,
	},
	cli.StringFlag{
		Name:        "microsoft-auth-tenant",
		Usage:       "(optional) Tenant for a microsoft OAuth application - Used for Microsoft Auth",
		EnvVar:      "MICROSOFT_AUTH_TENANT",
		Destination: &initialDeployArgs.MicrosoftAuthTenant,
	},
	cli.StringSliceFlag{
		Name:  "add-tag",
		Usage: "(optional) Key=Value pair to tag EC2 instances with - Multiple tags can be applied with multiple uses of this flag",
		Value: &initialDeployArgs.Tags,
	},
	cli.StringFlag{
		Name:        "namespace",
		Usage:       "(optional) Specify a namespace for deployments in order to group them in a meaningful way",
		EnvVar:      "NAMESPACE",
		Destination: &initialDeployArgs.Namespace,
	},
	cli.StringFlag{
		Name:        "zone",
		Usage:       "(optional) Specify an availability zone",
		EnvVar:      "ZONE",
		Destination: &initialDeployArgs.Zone,
	},
	cli.StringFlag{
		Name:        "vpc-network-range",
		Usage:       "(optional) VPC network CIDR to deploy into, only required if IAAS is AWS",
		EnvVar:      "VPC_NETWORK_RANGE",
		Destination: &initialDeployArgs.NetworkCIDR,
	},
	cli.StringFlag{
		Name:        "public-subnet-range",
		Usage:       "(optional) public network CIDR (if IAAS is AWS must be within --vpc-network-range)",
		EnvVar:      "PUBLIC_SUBNET_RANGE",
		Destination: &initialDeployArgs.PublicCIDR,
	},
	cli.StringFlag{
		Name:        "private-subnet-range",
		Usage:       "(optional) private network CIDR (if IAAS is AWS must be within --vpc-network-range)",
		EnvVar:      "PRIVATE_SUBNET_RANGE",
		Destination: &initialDeployArgs.PrivateCIDR,
	},
	cli.StringFlag{
		Name:        "rds-subnet-range1",
		Usage:       "(optional) first rds network CIDR (if IAAS is AWS must be within --vpc-network-range)",
		EnvVar:      "RDS_SUBNET_RANGE1",
		Destination: &initialDeployArgs.RDS1CIDR,
	},
	cli.StringFlag{
		Name:        "rds-subnet-range2",
		Usage:       "(optional) second rds network CIDR (if IAAS is AWS must be within --vpc-network-range)",
		EnvVar:      "RDS_SUBNET_RANGE2",
		Destination: &initialDeployArgs.RDS2CIDR,
	},
}

func deployAction(c *cli.Context, deployArgs deploy.Args, provider iaas.Provider) error {
	name := c.Args().Get(0)
	if name == "" {
		return errors.New("Usage is `control-tower deploy <name>`")
	}

	version := c.App.Version

	deployArgs, err := setZoneAndRegion(provider.Region(), deployArgs)
	if err != nil {
		return err
	}

	err = validateNameLength(name, provider.IAAS())
	if err != nil {
		return err
	}

	err = validateCidrRanges(provider, deployArgs.NetworkCIDR, deployArgs.PublicCIDR, deployArgs.PrivateCIDR, deployArgs.RDS1CIDR, deployArgs.RDS2CIDR)
	if err != nil {
		return err
	}

	client, err := buildClient(name, version, deployArgs, provider)
	if err != nil {
		return err
	}

	return client.Deploy()
}

func validateDeployArgs(c *cli.Context, deployArgs deploy.Args) (deploy.Args, error) {
	err := deployArgs.MarkSetFlags(c)
	if err != nil {
		return deployArgs, fmt.Errorf("failed to mark set Deploy flags: [%v]", err)
	}

	if err = deployArgs.Validate(); err != nil {
		return deployArgs, fmt.Errorf("failed to validate Deploy flags: [%v]", err)
	}

	return deployArgs, nil
}

func setZoneAndRegion(providerRegion string, deployArgs deploy.Args) (deploy.Args, error) {
	if !deployArgs.RegionIsSet {
		deployArgs.Region = providerRegion
	}

	if deployArgs.ZoneIsSet && deployArgs.RegionIsSet {
		if err := zoneBelongsToRegion(deployArgs.Zone, deployArgs.Region); err != nil {
			return deployArgs, err
		}
	}

	if deployArgs.ZoneIsSet && !deployArgs.RegionIsSet {
		region, message := regionFromZone(deployArgs.Zone)
		if region != "" {
			deployArgs.Region = region
			fmt.Print(message)
		}
	}

	return deployArgs, nil
}

func regionFromZone(zone string) (string, string) {
	re := regexp.MustCompile(`(?m)^\w+-\w+-\d`)
	regionFound := re.FindString(zone)
	if regionFound != "" {
		return regionFound, fmt.Sprintf("No region provided, please note that your zone will be paired with a matching region.\nThis region: %s is used for deployment.\n", regionFound)
	}
	return "", ""
}

func zoneBelongsToRegion(zone, region string) error {
	if !strings.Contains(zone, region) {
		return fmt.Errorf("The region and the zones provided do not match. Please note that the zone %s needs to be within a %s region", zone, region)
	}
	return nil
}

func validateNameLength(name string, providerName iaas.Name) error {
	if providerName == iaas.GCP {
		if len(name) > maxAllowedNameLength {
			return fmt.Errorf("deployment name %s is too long. %d character limit", name, maxAllowedNameLength)
		}
	}

	return nil
}

func validateCidrRanges(provider iaas.Provider, networkCIDR, publicCIDR, privateCIDR, RDS1CIDR, RDS2CIDR string) error {
	var parsedNetworkCidr, parsedPublicCidr, parsedPrivateCidr, parsedRDS1CIDR, parsedRDS2CIDR *net.IPNet
	var err error

	if networkCIDR == "" && publicCIDR == "" && privateCIDR == "" && RDS1CIDR == "" && RDS2CIDR == "" {
		return nil
	}

	if provider.IAAS() == iaas.AWS {
		if (privateCIDR != "" || publicCIDR != "" || RDS1CIDR != "" || RDS2CIDR != "") && networkCIDR == "" {
			return errors.New("error validating CIDR ranges - vpc-network-range must be provided when using AWS")
		}
		_, parsedNetworkCidr, err = net.ParseCIDR(networkCIDR)
		if err != nil {
			return errors.New("error validating CIDR ranges - vpc-network-range is not a valid CIDR")
		}
		if !validateNetworkSize(parsedNetworkCidr) {
			return errors.New("error validating CIDR ranges - vpc-network-range is not big enough, at least /26 needed.")
		}
		if RDS1CIDR == "" || RDS2CIDR == "" {
			return errors.New("error validating CIDR ranges - both rds1-subnet-range and rds2-subnet-range must be provided")
		}
		_, parsedRDS1CIDR, err = net.ParseCIDR(RDS1CIDR)
		if err != nil {
			return errors.New("error validating CIDR ranges - rds1-subnet-range is not a valid CIDR")
		}
		if !validateRDSSubnetSize(parsedRDS1CIDR) {
			return errors.New("error validating CIDR ranges - rds1-subnet-range is not big enough, at least /29 needed.")
		}
		_, parsedRDS2CIDR, err = net.ParseCIDR(RDS2CIDR)
		if err != nil {
			return errors.New("error validating CIDR ranges - rds2-subnet-range is not a valid CIDR")
		}
		if !validateRDSSubnetSize(parsedRDS2CIDR) {
			return errors.New("error validating CIDR ranges - rds2-subnet-range is not big enough, at least /29 needed.")
		}

	}
	if privateCIDR != "" || publicCIDR != "" {
		if privateCIDR == "" || publicCIDR == "" {
			return errors.New("error validating CIDR ranges - both public-subnet-range and private-subnet-range must be provided")
		}
	}
	_, parsedPublicCidr, err = net.ParseCIDR(publicCIDR)
	if err != nil {
		return errors.New("error validating CIDR ranges - public-subnet-range is not a valid CIDR")
	}
	if !validateSubnetSize(parsedPublicCidr) {
		return errors.New("error validating CIDR ranges - public-subnet-range is not big enough, at least /28 needed.")
	}
	_, parsedPrivateCidr, err = net.ParseCIDR(privateCIDR)
	if err != nil {
		return errors.New("error validating CIDR ranges - private-subnet-range is not a valid CIDR")
	}
	if !validateSubnetSize(parsedPrivateCidr) {
		return errors.New("error validating CIDR ranges - private-subnet-range is not big enough, at least /28 needed.")
	}

	if provider.IAAS() == iaas.AWS {
		if !parsedNetworkCidr.Contains(parsedPublicCidr.IP) {
			return errors.New("error validating CIDR ranges - public-subnet-range must be within vpc-network-range")
		}

		if !parsedNetworkCidr.Contains(parsedPrivateCidr.IP) {
			return errors.New("error validating CIDR ranges - private-subnet-range must be within vpc-network-range")
		}

		if !parsedNetworkCidr.Contains(parsedRDS1CIDR.IP) {
			return errors.New("error validating CIDR ranges - rds1-subnet-range must be within vpc-network-range")
		}

		if !parsedNetworkCidr.Contains(parsedRDS2CIDR.IP) {
			return errors.New("error validating CIDR ranges - rds2-subnet-range must be within vpc-network-range")
		}

		if parsedPublicCidr.Contains(parsedPrivateCidr.IP) || parsedPrivateCidr.Contains(parsedPublicCidr.IP) {
			return errors.New("error validating CIDR ranges - public-subnet-range must not overlap private-network-range")
		}
	}

	return nil
}

func cidrSize(cidr *net.IPNet) float64 {
	prefix, suffix := cidr.Mask.Size()
	return math.Pow(2, float64(suffix-prefix))
}

func validateNetworkSize(cidr *net.IPNet) bool {
	size := cidrSize(cidr)
	return size > 16
}

func validateSubnetSize(cidr *net.IPNet) bool {
	size := cidrSize(cidr)
	return size > 8
}

func validateRDSSubnetSize(cidr *net.IPNet) bool {
	size := cidrSize(cidr)
	return size > 4
}

func buildClient(name, version string, deployArgs deploy.Args, provider iaas.Provider) (*concourse.Client, error) {
	versionFile, _ := provider.Choose(iaas.Choice{
		AWS: resource.AWSVersionFile,
		GCP: resource.GCPVersionFile,
	}).([]byte)

	terraformClient, err := terraform.New(provider.IAAS(), terraform.DownloadTerraform(versionFile))
	if err != nil {
		return nil, err
	}

	tfInputVarsFactory, err := concourse.NewTFInputVarsFactory(provider)
	if err != nil {
		return nil, fmt.Errorf("Error creating TFInputVarsFactory [%v]", err)
	}

	client := concourse.NewClient(
		provider,
		terraformClient,
		tfInputVarsFactory,
		bosh.New,
		fly.New,
		certs.Generate,
		config.New(provider, name, deployArgs.Namespace),
		&deployArgs,
		os.Stdout,
		os.Stderr,
		util.FindUserIP,
		certs.NewAcmeClient,
		util.GeneratePasswordWithLength,
		util.EightRandomLetters,
		util.GenerateSSHKeyPair,
		version,
		versionFile,
	)

	return client, nil
}

var deployCmd = cli.Command{
	Name:      "deploy",
	Aliases:   []string{"d"},
	Usage:     "Deploys or updates a Concourse",
	ArgsUsage: "<name>",
	Flags:     deployFlags,
	Action: func(c *cli.Context) error {
		deployArgs, err := validateDeployArgs(c, initialDeployArgs)
		if err != nil {
			return fmt.Errorf("Error validating args on deploy: [%v]", err)
		}
		iaasName, err := iaas.Validate(deployArgs.IAAS)
		if err != nil {
			return fmt.Errorf("Error mapping to supported IAASes on deploy: [%v]", err)
		}
		provider, err := iaas.New(iaasName, deployArgs.Region)
		if err != nil {
			return fmt.Errorf("Error creating IAAS provider on deploy: [%v]", err)
		}
		return deployAction(c, deployArgs, provider)
	},
}
