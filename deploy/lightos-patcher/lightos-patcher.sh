#!/usr/bin/env bash
set -e

######## Colors ############
black=0; red=1; green=2; yellow=3; blue=4; pink=5; cyan=6; white=7;
cecho () {
  local _color=$1; shift
  echo -e "$(tput setaf $_color)$@$(tput sgr0)"
}

# wrapping functions
err () {
  cecho 1 "$@" >&2;
}

info () {
  cecho 2 "$@" >&2;
}

warn () {
  cecho 3 "$@" >&2;
}

debug () {
  cecho 6 "$@" >&2;
}
############################

STORAGE_CLASS=""
SNAPSHOT_STORAGE_CLASS=""
NEW_MGMT_EP=""
DEST_FOLDER=""

function usage() {
        info "Usage: ${0##*/} [-s <storage_class>] [-e <endpoints>] [-d <backup_directory>]"
        info "-v <storage_class>		name of the storage class and all related PVs to update"
        info "-s <snapshot_storage_class>	name of the snapshot storage class and all related SnapshotContents to update"
        info "-e <endpoints>			new endpoint list in the form of: <host:port>,<host:port>,..."
        info "-d <backup_directory>		folder to backup before and after resources"
        debug "Examples:"
        debug ""
    	debug "		suppose we have LightOS Cluster los1 with the following mgmt-endpoints:"
    	debug "		192.168.17.2:443,192.168.18.3:443,192.168.20.4:443,192.168.20.5:443"
        debug ""
    	debug "		After extending this cluster by adding a new server (192.168.20.5:443) we will have the following mgmt-endpoints:"
    	debug "		192.168.17.2:443,192.168.18.3:443,192.168.20.4:443,192.168.20.5:443"
        debug ""
    	debug "		# patch example-sc StorageClass and all PVs related to that StorageClass"
    	debug "		./lightos-patcher.sh -v example-sc -e 192.168.17.2:443,192.168.18.3:443,192.168.20.4:443,192.168.20.5:443 -d ~/back"
        debug ""
    	debug "		# patch example-sc VolumeSnapshotClass and all VolumeSnapshotContents related to that class"
    	debug "		./lightos-patcher.sh -s example-snap-sc -e 192.168.17.2:443,192.168.18.3:443,192.168.20.4:443,192.168.20.5:443 -d ~/backup"
}

while getopts "v:s:e:d:h-:" opt; do
        case "${opt}" in
                -)
                        echo "  Invalid argument: $OPTARG"
                        usage
                        exit 1
                ;;
                v)
                        STORAGE_CLASS=$OPTARG
                ;;
                s)
                        SNAPSHOT_STORAGE_CLASS=$OPTARG
                ;;
                e)
                        NEW_MGMT_EP=$OPTARG
                ;;
                d)
                        DEST_FOLDER=$OPTARG
                ;;
                h)
                        usage
                        exit 0
                ;;
                *)
                        echo "  Invalid argument: $OPTARG"
                        usage
                        exit 1
                ;;
        esac
done


if [ -z "$STORAGE_CLASS" ] && [ -z "$SNAPSHOT_STORAGE_CLASS" ]; then
	err "STORAGE_CLASS and SNAPSHOT_STORAGE_CLASS environment variables not set and -v/-s options not used"
	usage
	exit 1
fi

if [ -z "$NEW_MGMT_EP" ]; then
	err "NEW_MGMT_EP environment variable not set and -e option not used"
	usage
	exit 1
fi

if [ -z "$DEST_FOLDER" ]; then
	err "DEST_FOLDER environment variable not set and -d option not used"
	usage
	exit 1
fi

function patch_sc()
{
	sc_name="${1}"
	sc_mgmt_ep="${2}"

	if [[ "$mgmt_ep" != "$NEW_MGMT_EP" ]]; then
		sc_file_name="$DEST_FOLDER/$sc_name.yaml"
		info "sc_name: $sc_name. stored backup file: $sc_file_name"
		kubectl get sc $sc_name -oyaml > $sc_file_name
		#echo "******** before $sc_name ********"
		#cat $sc_file_name
		#echo "*******************************"

		sed -i.bak "s/mgmt-endpoint: .*/mgmt-endpoint: $NEW_MGMT_EP/g" $sc_file_name
		# echo "******** after $sc_name ********"
		# cat $sc_file_name
		# echo "*******************************"

		kubectl replace -f $sc_file_name --force
	else
		echo "parameters.mgmt-endpoint in $sc_name match - skipping."
	fi
}

function patch_pv()
{
	pv_name="${1}"
	pv_volume_handle="${2}"

	mgmt_ep=$(echo $pv_volume_handle | grep -oP "mgmt:\K([0-9:,\.]*)|")
	if [[ "$mgmt_ep" != "$NEW_MGMT_EP" ]]; then

		new_volume_handle=$(sed "s/$mgmt_ep/$NEW_MGMT_EP/g" <<< "$pv_volume_handle")

		pv_file_name="$DEST_FOLDER/$pv_name.yaml"
		info "pv_name: $pv_name. stored backup file: $pv_file_name"
		kubectl get pv $pv_name -oyaml > $pv_file_name
		#echo "******** before $pv_name ********"
		#cat $pv_file_name
		#echo "*******************************"

		sed -i.bak "s/volumeHandle: .*/volumeHandle: $new_volume_handle/g" $pv_file_name
		# echo "******** after $pv_name ********"
		# cat $pv_file_name
		# echo "*******************************"

		# replace may be a blocking call if the PV has finalizers.
		# we need to let replace at least delete the PV, it will move to status="Terminating" only then we should patch
		# and remove the finalizers.
		kubectl replace -f $pv_file_name --force &
		replace_pid=$!

		# note: for some reason when PV is in terminating state, the status.phase is still Bound
		# there is no place to get the Terminating status from - the only way to deduce this state is to ask
		# about deletion-ts. we run this poller while string is empty
		while [ -z "$(kubectl get pv $pv_name -o jsonpath='{.metadata.deletionTimestamp}')" ]; do
			sleep 1
			echo "Waiting for pv $pv_name to become terminating"
		done

		echo "removing finalizers from $pv_name"
		kubectl patch pv $pv_name -p '{"metadata":{"finalizers":null}}'
		wait $replace_pid
		echo "replace command on $pv_name ended"
	else
		echo "$pv_name mgmt_eps match - skipping."
	fi
}

function patch_dynamic_provisioned_volume_snapshot_content()
{
	vsc_name="${1}"
	vs_name="${2}"
	vsc_volume_handle="${3}"
	vsc_snapshot_handle="${4}"
	
	mgmt_ep=$(echo $vsc_snapshot_handle | grep -oP "mgmt:\K([0-9:,\.]*)|")
	if [[ "$mgmt_ep" != "$NEW_MGMT_EP" ]]; then

		new_vsc_snapshot_handle=$(sed "s/$mgmt_ep/$NEW_MGMT_EP/g" <<< "$vsc_snapshot_handle")

		kubectl patch --type=merge volumesnapshotcontent $vsc_name -p "{\"spec\": {\"source\": {\"snapshotHandle\": \"$new_vsc_snapshot_handle\", \"volumeHandle\": null}}}"

		kubectl patch --type=merge volumesnapshot $vs_name -p "{\"spec\": {\"source\": {\"volumeSnapshotContentName\": \"$vsc_name\", \"persistentVolumeClaimName\": null}}}"


		vsc_file_name="$DEST_FOLDER/$vsc_name.yaml"
		info "vsc_name: $vsc_name. stored backup file: $vsc_file_name"
		kubectl get volumesnapshotcontent $vsc_name -oyaml > $vsc_file_name
		#echo "******** before $vsc_name ********"
		#cat $vsc_file_name
		#echo "*******************************"

		kubectl patch --type=merge volumesnapshotcontent $vsc_name -p "{\"spec\":{\"deletionPolicy\":\"Retain\"}}"
		# delete may be a blocking call if the PV has finalizers.
		# deleting will move the volumesnapshotcontent to status="Terminating" only then we should patch
		# and remove the finalizers.
		kubectl delete volumesnapshotcontent $vsc_name &
		delete_pid=$!

		# note: for some reason when PV is in terminating state, the status.phase is still Bound
		# there is no place to get the Terminating status from - the only way to deduce this state is to ask
		# about deletion-ts. we run this poller while string is empty
		while [ -z "$(kubectl get volumesnapshotcontent $vsc_name -o jsonpath='{.metadata.deletionTimestamp}')" ]; do
			sleep 1
			echo "Waiting for volumesnapshotcontent $vsc_name to become terminating"
		done

		echo "removing finalizers from $vsc_name"
		kubectl patch --type merge volumesnapshotcontent $vsc_name -p '{"metadata": {"finalizers": null}}'
		wait $delete_pid
		echo "delete command on $vsc_name ended. creating it again to update status.snapshotHandle"
		kubectl create -f $vsc_file_name

	else
		echo "$vsc_name mgmt_eps match - skipping."
	fi
}

function patch_pre_provisioned_volume_snapshot_content()
{
	vsc_name="${1}"
	vs_name="${2}"
	vsc_snapshot_handle="${3}"

	mgmt_ep=$(echo $vsc_snapshot_handle | grep -oP "mgmt:\K([0-9:,\.]*)|")
	if [[ "$mgmt_ep" != "$NEW_MGMT_EP" ]]; then

		new_vsc_snapshot_handle=$(sed "s/$mgmt_ep/$NEW_MGMT_EP/g" <<< "$vsc_snapshot_handle")

		kubectl patch --type=merge volumesnapshotcontent $vsc_name -p "{\"spec\": {\"source\": {\"snapshotHandle\": \"$new_vsc_snapshot_handle\", \"volumeHandle\": null}}}"

		# kubectl patch --type=merge volumesnapshot $vs_name -p "{\"spec\": {\"source\": {\"volumeSnapshotContentName\": \"$vsc_name\", \"persistentVolumeClaimName\": null}}}"


		vsc_file_name="$DEST_FOLDER/$vsc_name.yaml"
		info "vsc_name: $vsc_name. stored backup file: $vsc_file_name"
		kubectl get volumesnapshotcontent $vsc_name -oyaml > $vsc_file_name
		#echo "******** before $vsc_name ********"
		#cat $vsc_file_name
		#echo "*******************************"

		kubectl patch --type=merge volumesnapshotcontent $vsc_name -p "{\"spec\":{\"deletionPolicy\":\"Retain\"}}"
		# delete may be a blocking call if the PV has finalizers.
		# deleting will move the volumesnapshotcontent to status="Terminating" only then we should patch
		# and remove the finalizers.
		kubectl delete volumesnapshotcontent $vsc_name &
		delete_pid=$!

		# note: for some reason when PV is in terminating state, the status.phase is still Bound
		# there is no place to get the Terminating status from - the only way to deduce this state is to ask
		# about deletion-ts. we run this poller while string is empty
		while [ -z "$(kubectl get volumesnapshotcontent $vsc_name -o jsonpath='{.metadata.deletionTimestamp}')" ]; do
			sleep 1
			echo "Waiting for volumesnapshotcontent $vsc_name to become terminating"
		done

		echo "removing finalizers from $vsc_name"
		kubectl patch --type merge volumesnapshotcontent $vsc_name -p '{"metadata": {"finalizers": null}}'
		wait $delete_pid
		echo "delete command on $vsc_name ended. creating it again to update status.snapshotHandle"
		kubectl create -f $vsc_file_name

	else
		echo "$vsc_name mgmt_eps match - skipping."
	fi
}


function patch_pvs_by_storage_class()
{
	storage_class_name=$1
	for sc_info in $(kubectl get sc $storage_class_name -o jsonpath='{.metadata.name}/{.parameters.mgmt-endpoint}{"\n"}');
	do
		IFS='/' read -r -a array <<< "$sc_info"
		sc_name="${array[0]}"
		sc_mgmt_ep="${array[1]}"

		patch_sc $sc_name $sc_mgmt_ep

		for pv_info in $(kubectl get pv -o jsonpath="{range.items[?(@.spec.storageClassName=='$sc_name')]}{.metadata.name}/{.spec.csi.volumeHandle}{\"\n\"}{end}");
		do
			IFS='/' read -r -a array <<< "$pv_info"
			pv_name="${array[0]}"
			pv_volume_handle="${array[1]}"

			patch_pv $pv_name $pv_volume_handle
		done

		echo ""
	done
}

# iterate over all snapshots in the volumesnapshotclass and invoke update for each one.
function patch_volume_snapshot_content_by_volume_snapshot_class()
{
	volume_snapshot_class_name=$1
	for sc_info in $(kubectl get volumesnapshotclasses $volume_snapshot_class_name -o jsonpath='{.metadata.name}/{.driver}{"\n"}');
	do
		IFS='/' read -r -a array <<< "$sc_info"
		sc_name="${array[0]}"
		driver_name="${array[1]}"

		if [ "$driver_name" != "csi.lightbitslabs.com" ]; then
			err "provided volume snapshot class not related to LightOS"
			exit 2
		fi

		for vsc_info in $(kubectl get volumesnapshotcontent -o jsonpath="{range.items[?(@.spec.volumeSnapshotClassName=='$sc_name')]}{.metadata.name}/{.spec.volumeSnapshotRef.name}/{.spec.source.volumeHandle}/{.status.snapshotHandle}{\"\n\"}{end}");
		do
			IFS='/' read -r -a array <<< "$vsc_info"
			vsc_name="${array[0]}"
			vs_name="${array[1]}"
			vsc_volume_handle="${array[2]}"
			vsc_snapshot_handle="${array[3]}"

			if [ -z "$vsc_snapshot_handle" ] && [ -z "$vsc_volume_handle" ]; then
				err "at least one of 'vsc_volume_handle' or 'vsc_snapshot_handle' must be set"
				exit 3
			fi

			if [ ! -z "$vsc_volume_handle" ]; then
				info "'vsc_volume_handle' is provided calling patch_dynamic_provisioned_volume_snapshot_content"
				patch_dynamic_provisioned_volume_snapshot_content $vsc_name $vs_name $vsc_volume_handle $vsc_snapshot_handle
			fi
			if [ ! -z "$vsc_snapshot_handle" ]; then
				info "'vsc_snapshot_handle' is provided calling patch_pre_provisioned_volume_snapshot_content"
				patch_pre_provisioned_volume_snapshot_content $vsc_name $vs_name $vsc_snapshot_handle
			fi
		done

		echo ""
	done
}


function patch_storage_classes()
{
	for sc_info in $(kubectl get sc -o jsonpath='{range.items[?(@.provisioner=="csi.lightbitslabs.com")]}{.metadata.name}/{.parameters.mgmt-endpoint}{"\n"}{end}');
	do
		IFS='/' read -r -a array <<< "$sc_info"
		sc_name="${array[0]}"
		sc_mgmt_ep="${array[1]}"

		patch_sc $sc_name $sc_mgmt_ep
		echo ""
	done
}

function patch_pvs()
{
	for pv_info in $(kubectl get pv -o jsonpath='{range.items[?(@.spec.csi.driver=="csi.lightbitslabs.com")]}{.metadata.name}/{.spec.csi.volumeHandle}{"\n"}{end}');
	do
		IFS='/' read -r -a array <<< "$pv_info"
		pv_name="${array[0]}"
		pv_volume_handle="${array[1]}"

		patch_pv $pv_name $pv_volume_handle
		echo ""
	done
}

######## menu #################

mkdir -p $DEST_FOLDER
if [ -n "$STORAGE_CLASS" ]; then
	patch_pvs_by_storage_class $STORAGE_CLASS
fi

if [ -n "$SNAPSHOT_STORAGE_CLASS" ]; then
	patch_volume_snapshot_content_by_volume_snapshot_class $SNAPSHOT_STORAGE_CLASS
fi

###############################
