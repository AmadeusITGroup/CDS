#!/bin/sh
# must be run as root

# We assume for now that sshd is installed
#zypper install -y openssh
ensureSSH() {
    if [ ! -e /usr/sbin/sshd ]; then
        echo "sshd does not exist"

        # Most RHEL based distributions
        if [ -e /usr/bin/dnf ]; then
            echo "dnf exist. installing sshd"
            /usr/bin/dnf -y install openssh openssh-server
            return
        fi

        # Fedora
        if [ -e /usr/bin/microdnf ]; then
            echo "microdnf exist. installing sshd"
            /usr/bin/microdnf -y install openssh openssh-server
            return
        fi

        if [ -e /usr/bin/zypper ]; then
            echo "zypper exist. installing sshd"
            /usr/bin/zypper install -y openssh
            return
        fi

        if [ -e /usr/bin/apt-get ]; then
            echo "apt-get exist. installing sshd"
            /usr/bin/apt-get update
            /usr/bin/apt-get -y install openssh-server
            # Prevent the error "Missing privilege separation directory: /run/sshd"
            mkdir -p /run/sshd
            return
        fi
    fi
}

# Start or restart the sshd process
restartSshd() {
    # Standard kill does not always terminate processes,
    # so the sshd may not be restarted as expected.
    # As consequence the changes won't be applied to the devcontainer.
    # Use kill -9 to ensure the process is killed.
    kill -9 $(pidof sshd) > /dev/null 2>&1
    /usr/sbin/sshd
}

# Set ulimit for core files to unlimited.
# See more on the man page: https://linux.die.net/man/5/limits.conf
ensureCoreFileUlimit() {
    # Ensure /etc/security/limits.d exists
    mkdir -p /etc/security/limits.d
    cat > /etc/security/limits.d/cds.conf << EOF
# This file was created by CDS to define the limit size of files inside the devcontainer.
# It follows the limits.conf man page: https://linux.die.net/man/5/limits.conf.

*               soft    core            unlimited
EOF
}

ensureCoreFileUlimit
ensureSSH

ssh-keygen -A

# Enable the transfer of local environment variable LC_VSCODEHOST
# into the development container
echo "AcceptEnv LC_VSCODEHOST" >> /etc/ssh/sshd_config

# start or restart sshd to load
# correctly the changes in sshd_config
restartSshd
