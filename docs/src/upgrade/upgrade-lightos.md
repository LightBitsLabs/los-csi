<div style="page-break-after: always;"></div>

## LightOS Cluster Upgrade

Follow the upgrade procedure of LightOS Cluster.

### Known Issues

1. There is a bug that we limit the yum install for `5m`. It might not be enough. In case this happens we should upgrade the UM manually using the following flow:

   ```bash
   cat > /etc/yum.repos.d/lightos-2.2.1.repo << EOF
   [lightos-2.2.1]
   name=lightos-2.2.1
   Baseurl=<repo_url>
   gpgcheck=0
   enabled=1
   sslverify=false
   EOF

   yum update --disablerepo=* --enablerepo=lightos-2.2.1 management-upgrade-manager
   systemctl daemon-reload
   systemctl stop upgrade-manager.service
   systemctl start upgrade-manager.service
   ```

2. Configuring the yum repo to work behind http-proxy.
