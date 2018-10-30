package api

import (
	"strconv"
	"strings"

	"github.com/Azure/aks-engine/pkg/api/common"
	"github.com/Azure/aks-engine/pkg/api/v20170930"
	"github.com/Azure/aks-engine/pkg/api/vlabs"
	"github.com/blang/semver"
	"github.com/pkg/errors"
)

type orchestratorsFunc func(*OrchestratorProfile, bool) ([]*OrchestratorVersionProfile, error)

var funcmap map[string]orchestratorsFunc
var versionsMap map[string][]string

func init() {
	funcmap = map[string]orchestratorsFunc{
		Kubernetes: kubernetesInfo,
	}
	versionsMap = map[string][]string{
		Kubernetes: common.GetAllSupportedKubernetesVersions(true, false),
	}
}

func validate(orchestrator, version string) (string, error) {
	switch {
	case strings.EqualFold(orchestrator, Kubernetes):
		return Kubernetes, nil
	case orchestrator == "":
		if version != "" {
			return "", errors.Errorf("Must specify orchestrator for version '%s'", version)
		}
	default:
		return "", errors.Errorf("Unsupported orchestrator '%s'", orchestrator)
	}
	return "", nil
}

func isVersionSupported(csOrch *OrchestratorProfile) bool {
	supported := false
	for _, version := range versionsMap[csOrch.OrchestratorType] {

		if version == csOrch.OrchestratorVersion {
			supported = true
			break
		}
	}
	return supported
}

// GetOrchestratorVersionProfileListVLabs returns vlabs OrchestratorVersionProfileList object per (optionally) specified orchestrator and version
func GetOrchestratorVersionProfileListVLabs(orchestrator, version string) (*vlabs.OrchestratorVersionProfileList, error) {
	apiOrchs, err := getOrchestratorVersionProfileList(orchestrator, version)
	if err != nil {
		return nil, err
	}
	orchList := &vlabs.OrchestratorVersionProfileList{}
	orchList.Orchestrators = []*vlabs.OrchestratorVersionProfile{}
	for _, orch := range apiOrchs {
		orchList.Orchestrators = append(orchList.Orchestrators, ConvertOrchestratorVersionProfileToVLabs(orch))
	}
	return orchList, nil
}

// GetOrchestratorVersionProfileListV20170930 returns v20170930 OrchestratorVersionProfileList object per (optionally) specified orchestrator and version
func GetOrchestratorVersionProfileListV20170930(orchestrator, version string) (*v20170930.OrchestratorVersionProfileList, error) {
	apiOrchs, err := getOrchestratorVersionProfileList(orchestrator, version)
	if err != nil {
		return nil, err
	}
	orchList := &v20170930.OrchestratorVersionProfileList{}
	for _, orch := range apiOrchs {
		orchList.Properties.Orchestrators = append(orchList.Properties.Orchestrators, ConvertOrchestratorVersionProfileToV20170930(orch))
	}
	return orchList, nil
}

func getOrchestratorVersionProfileList(orchestrator, version string) ([]*OrchestratorVersionProfile, error) {
	var err error
	if orchestrator, err = validate(orchestrator, version); err != nil {
		return nil, err
	}
	orchs := []*OrchestratorVersionProfile{}
	if len(orchestrator) == 0 {
		// return all orchestrators
		for _, f := range funcmap {
			arr, err := f(&OrchestratorProfile{}, false)
			if err != nil {
				return nil, err
			}
			orchs = append(orchs, arr...)
		}
	} else {
		if orchs, err = funcmap[orchestrator](&OrchestratorProfile{OrchestratorType: orchestrator, OrchestratorVersion: version}, false); err != nil {
			return nil, err
		}
	}
	return orchs, nil
}

// GetOrchestratorVersionProfile returns orchestrator info for upgradable container service
func GetOrchestratorVersionProfile(orch *OrchestratorProfile, hasWindows bool) (*OrchestratorVersionProfile, error) {
	if orch.OrchestratorVersion == "" {
		return nil, errors.New("Missing Orchestrator Version")
	}
	switch orch.OrchestratorType {
	case Kubernetes:
		arr, err := funcmap[orch.OrchestratorType](orch, hasWindows)
		if err != nil {
			return nil, err
		}
		// has to be exactly one element per specified orchestrator/version
		if len(arr) != 1 {
			return nil, errors.New("Ambiguous Orchestrator Versions")
		}
		return arr[0], nil
	default:
		return nil, errors.Errorf("Upgrade operation is not supported for '%s'", orch.OrchestratorType)
	}
}

func kubernetesInfo(csOrch *OrchestratorProfile, hasWindows bool) ([]*OrchestratorVersionProfile, error) {
	orchs := []*OrchestratorVersionProfile{}
	if csOrch.OrchestratorVersion == "" {
		// get info for all supported versions
		for _, ver := range common.GetAllSupportedKubernetesVersions(false, hasWindows) {
			upgrades, err := kubernetesUpgrades(&OrchestratorProfile{OrchestratorVersion: ver}, hasWindows)
			if err != nil {
				return nil, err
			}
			orchs = append(orchs,
				&OrchestratorVersionProfile{
					OrchestratorProfile: OrchestratorProfile{
						OrchestratorType:    Kubernetes,
						OrchestratorVersion: ver,
					},
					Default:  ver == common.GetDefaultKubernetesVersion(hasWindows),
					Upgrades: upgrades,
				})
		}
	} else {
		if !isVersionSupported(csOrch) {
			return nil, errors.Errorf("Kubernetes version %s is not supported", csOrch.OrchestratorVersion)
		}

		upgrades, err := kubernetesUpgrades(csOrch, hasWindows)
		if err != nil {
			return nil, err
		}
		orchs = append(orchs,
			&OrchestratorVersionProfile{
				OrchestratorProfile: OrchestratorProfile{
					OrchestratorType:    Kubernetes,
					OrchestratorVersion: csOrch.OrchestratorVersion,
				},
				Default:  csOrch.OrchestratorVersion == common.GetDefaultKubernetesVersion(hasWindows),
				Upgrades: upgrades,
			})
	}
	return orchs, nil
}

func kubernetesUpgrades(csOrch *OrchestratorProfile, hasWindows bool) ([]*OrchestratorProfile, error) {
	ret := []*OrchestratorProfile{}

	currentVer, err := semver.Make(csOrch.OrchestratorVersion)
	if err != nil {
		return nil, err
	}
	nextNextMinorString := strconv.FormatUint(currentVer.Major, 10) + "." + strconv.FormatUint(currentVer.Minor+2, 10) + ".0-alpha.0"
	upgradeableVersions := common.GetVersionsBetween(common.GetAllSupportedKubernetesVersions(false, hasWindows), csOrch.OrchestratorVersion, nextNextMinorString, false, true)
	for _, ver := range upgradeableVersions {
		ret = append(ret, &OrchestratorProfile{
			OrchestratorType:    Kubernetes,
			OrchestratorVersion: ver,
		})
	}
	return ret, nil
}
