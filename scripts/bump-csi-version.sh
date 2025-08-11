#!/bin/bash

repo_name=los-csi
repo_path=$WORKSPACE_TOP/$repo_name

function print_help {
    echo "Usage: $0 --csi_plugin_version <csi_plugin_version> --helm_chart_version <helm_chart_version> --cluster_version <cluster_version> --supported_k8s_version <supported_k8s_version> --release_date <release_date>"
    echo "Example: $0 --csi_plugin_version 1.18.0 --helm_chart_version 0.16.0 --cluster_version 3.13.1 --supported_k8s_version 1.30 --release_date 2024-12-23"
    echo "Note: The release date must be in the format YYYY-MM-DD"
}

function is_correct_version {
    local version=$1
    if [[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        return 0
    else
        return 1
    fi
}

function is_correct_k8s_version {
    local version=$1
    if [[ $version =~ ^[0-9]+\.[0-9]+$ ]]; then
        return 0
    else
        return 1
    fi
}

function is_valid_release_date {
    local date=$1
    if [[ $date =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        return 0
    else
        return 1
    fi
}


while [[ "$#" -gt 0 ]]; do
    case $1 in
        --csi_plugin_version) csi_plugin_version="$2"; shift ;;
        --helm_chart_version) helm_chart_version="$2"; shift ;;
        --cluster_version) cluster_version="$2"; shift ;;
        --supported_k8s_version) supported_k8s_version="$2"; shift ;;
        --release_date) release_date="$2"; shift ;;
        --help|-h) print_help; exit 0 ;;
        *) echo "Unknown parameter passed: $1"; print_help; exit 1 ;;
    esac
    shift
done

if [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
    print_help
    exit 0
fi

if ! is_correct_version "$csi_plugin_version"; then
    echo "Invalid csi_plugin_version format: $csi_plugin_version"
    print_help
    exit 1
fi

if ! is_correct_version "$helm_chart_version"; then
    echo "Invalid helm_chart_version format: $helm_chart_version"
    print_help
    exit 1
fi

if ! is_correct_version "$cluster_version"; then
    echo "Invalid cluster_version format: $cluster_version"
    print_help
    exit 1
fi

if ! is_correct_k8s_version "$supported_k8s_version"; then
    echo "Invalid supported_k8s_version format: $supported_k8s_version"
    print_help
    exit 1
fi

if ! is_valid_release_date "$release_date"; then
    echo "Invalid release date: $release_date"
    print_help
    exit 1
fi

if [ -z "$WORKSPACE_TOP" ] ; then echo "WORKSPACE_TOP not set, please source .env" ; exit 1 ; fi

csi_plugin_version=v$csi_plugin_version

sed -i "s/version:.*/version: $helm_chart_version/" $repo_path/deploy/helm/lb-csi/Chart.yaml
sed -i "s/appVersion:.*/appVersion: $csi_plugin_version/" $repo_path/deploy/helm/lb-csi/Chart.yaml

sed -i "s/image:.*/image: \"lb-csi-plugin:$csi_plugin_version\"/" $repo_path/deploy/helm/lb-csi/values.yaml
sed -i "s/discoveryClientImage:.*/discoveryClientImage: \"lb-nvme-discovery-client:$csi_plugin_version\"/" $repo_path/deploy/helm/lb-csi/values.yaml

sed -i "s/version:.*/version: $helm_chart_version/" $repo_path/deploy/helm/lb-csi-workload-examples/Chart.yaml
sed -i "s/appVersion:.*/appVersion: $csi_plugin_version/" $repo_path/deploy/helm/lb-csi-workload-examples/Chart.yaml
sed -i "s/discoveryClientImage:.*/discoveryClientImage: \"lb-nvme-discovery-client:$csi_plugin_version\"/" $repo_path/deploy/helm/lb-csi-workload-examples/Chart.yaml



#docs/metadata.md
sed -i "s/Lightbits CSI Plugin v.*/Lightbits CSI Plugin $csi_plugin_version Deployment Guide/" $repo_path/docs/metadata.md
sed -i "s/LightOS Version: .*/LightOS Version: v$cluster_version/" $repo_path/docs/metadata.md
sed -i "s/Kubernetes Versions: .*/Kubernetes Versions: v1.31 - v$supported_k8s_version/" $repo_path/docs/metadata.md



touch $repo_path/docs/src/CHANGELOG/CHANGELOG-$csi_plugin_version.md

cat << EOF > $repo_path/docs/src/CHANGELOG/CHANGELOG-$csi_plugin_version.md
<div style="page-break-after: always;"></div>

## $csi_plugin_version

Date: $release_date

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/$csi_plugin_version

### Container Image

docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:$csi_plugin_version

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:$helm_chart_version
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:$helm_chart_version

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/$csi_plugin_version/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/$csi_plugin_version/docs/src/upgrade

### Highlights



EOF


echo "- [CHANGELOG-$csi_plugin_version.md](./CHANGELOG-$csi_plugin_version.md)" >> $repo_path/docs/src/CHANGELOG/README.md


awk '/CHANGELOG/ {last_match=NR} END {print last_match}' $repo_path/docs/src/SUMMARY.md | xargs -I {} sed -i "{}a \ \ - [CHANGELOG-$csi_plugin_version](CHANGELOG/CHANGELOG-$csi_plugin_version.md)" $repo_path/docs/src/SUMMARY.md


sed -i "s/compatible with Lightbits™ version .*/compatible with Lightbits™ version $cluster_version./" $repo_path/docs/src/introduction.md
sed -i "s/If you upgrade the Lightbits cluster to version .*/If you upgrade the Lightbits cluster to version $cluster_version it is recommended to upgrade the CSI plugin to version $csi_plugin_version as well./" $repo_path/docs/src/introduction.md


sed -i "s/lb-csi-plugin-[0-9.]*.tgz/lb-csi-plugin-$csi_plugin_version.tgz/" $repo_path/docs/src/plugin_deployment/deployment.md
sed -i "s/lb-csi-workload-examples-[0-9.]*.tgz/lb-csi-workload-examples-$csi_plugin_version.tgz/" $repo_path/docs/src/plugin_deployment/deployment.md
sed -i "s/lb-csi-plugin:.*/lb-csi-plugin:$csi_plugin_version/" $repo_path/docs/src/plugin_deployment/deployment.md


sed -i -E "s/(lb-csi-plugin-)[0-9]+\.[0-9]+\.[0-9]+/\1${helm_chart_version}/" $repo_path/docs/src/plugin_deployment/plugin_deployment_using_chart_in_bundle.md
sed -i -E "s/(lb-csi\s+kube-system\s+1\s+)[0-9]{4}-[0-9]{2}-[0-9]{2}/\1${release_date}/" $repo_path/docs/src/plugin_deployment/plugin_deployment_using_chart_in_bundle.md
sed -i "s/\(.*\)[[:space:]]v[0-9]\+\.[0-9]\+\.[0-9]\+$/\1 ${csi_plugin_version}/" $repo_path/docs/src/plugin_deployment/plugin_deployment_using_chart_in_bundle.md

sed -i -E "s/(lightbits-helm-repo\/lb-csi-plugin\s+)[0-9]+\.[0-9]+\.[0-9]+(\s+)v[0-9]+\.[0-9]+\.[0-9]+/\1${helm_chart_version}\2${csi_plugin_version}/" $repo_path/docs/src/plugin_deployment/plugin_deployment_using_lightbits_helm_repository.md
sed -i -E "s/(lightbits-helm-repo\/lb-csi-workload-examples\s+)[0-9]+\.[0-9]+\.[0-9]+(\s+)v[0-9]+\.[0-9]+\.[0-9]+/\1${helm_chart_version}\2${csi_plugin_version}/" $repo_path/docs/src/plugin_deployment/plugin_deployment_using_lightbits_helm_repository.md
#sed -i -E "s/(lightbits-helm-repo\/snapshot-controller-3\s+)[0-9]+\.[0-9]+\.[0-9]+/\/" $repo_path/docs/src/plugin_deployment/plugin_deployment_using_lightbits_helm_repository.md
#sed -i -E "s/(lightbits-helm-repo\/snapshot-controller-4\s+)[0-9]+\.[0-9]+\.[0-9]+/\1${helm_chart_version}/" $repo_path/docs/src/plugin_deployment/plugin_deployment_using_lightbits_helm_repository.md