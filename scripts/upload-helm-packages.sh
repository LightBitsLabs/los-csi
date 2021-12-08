#!/usr/bin/env bash

# go over all charts and upload to repository
if [ -z "$HELM_CHART_REPOSITORY" ] ; then echo "HELM_CHART_REPOSITORY not set, can't push" ; exit 1 ; fi
if [ -z "$HELM_CHART_REPOSITORY_USERNAME" ] ; then echo "HELM_CHART_REPOSITORY_USERNAME not set, can't push" ; exit 1 ; fi
if [ -z "$HELM_CHART_REPOSITORY_PASSWORD" ] ; then echo "HELM_CHART_REPOSITORY_PASSWORD not set, can't push" ; exit 1 ; fi

regex='\./deploy/helm/charts/(lb-csi-plugin|lb-csi-workload-examples)-([0-9]+.[0-9]+.[0-9]+).tgz'
for FILE in ./deploy/helm/charts/*.tgz; do
	[[ $FILE =~ $regex ]]
	pkg_name=${BASH_REMATCH[1]}
	pkg_version=${BASH_REMATCH[2]}
	url="$HELM_CHART_REPOSITORY/api/charts/$pkg_name/$pkg_version"
	if (curl  -XGET -L -u $HELM_CHART_REPOSITORY_USERNAME:$HELM_CHART_REPOSITORY_PASSWORD -o/dev/null -sfI "$url"); then
	    echo "URL already exists. do nothing."
	else
	    echo "URL does not exist. upload..."
	    curl -XPOST -L -u $HELM_CHART_REPOSITORY_USERNAME:$HELM_CHART_REPOSITORY_PASSWORD -T $FILE $HELM_CHART_REPOSITORY/api/charts; echo;
	fi
done;
