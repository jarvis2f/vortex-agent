#!/bin/bash

vortex_agent_version="0.1.0"
gost_version="3.0.0-nightly.20240201"

vortex_agent_id=""
vortex_agent_key=""
vortex_server=""

function check_root() {
  if [[ $EUID != 0 ]]; then
    echo ">>> Please execute with root privileges."
    exit 1
  fi
  sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"
}

get_arch() {
  arch=$(uname -m)
  case $arch in
  "x86_64") arch="amd64" ;;
  "aarch64") arch="arm64" ;;
  *)
    echo ">>> Unsupported architecture: $arch"
    exit 1
    ;;
  esac
}

download() {
  local url=$1
  local name=$2
  bin_tar_name=$(echo "$url" | awk -F '/' '{print $NF}')

  echo ">>> Downloading $name from $url"
  if wget -nv -O "$bin_tar_name" "$url"; then
    tar -zxvf "$bin_tar_name"
    mv -f "$name" /usr/bin/
    echo ">>> Downloaded $name successfully"
    rm "$bin_tar_name" # Clean up temporary file after download
  else
    echo ">>> Failed to download $name"
    exit 1
  fi
}

download_script() {
  local url=$1
  echo ">>> Downloading script from $url"
  local file_name
  file_name=$(echo "$url" | awk -F '/' '{print $NF}')
  if wget -nv -O "$file_name" "$url" && chmod +x "$file_name"; then
    echo ">>> Script downloaded successfully"
  else
    echo ">>> Failed to download script"
    exit 1
  fi
}

install_vortex_agent() {
  if [ -f "/usr/bin/vortex" ]; then
    local installed_version
    installed_version=$(vortex -v | awk '{print $3}')
    echo ">>> vortex Installed version: $installed_version"
    echo ">>> vortex Required version: v$vortex_agent_version"
    if [ "$installed_version" = "v$vortex_agent_version" ]; then
      echo ">>> vortex already installed"
      return
    fi
  fi
  local url="https://github.com/jarvis2f/vortex-agent/releases/download/${vortex_agent_version}/vortex-agent_${vortex_agent_version}_linux_$arch.tar.gz"
  download "$url" vortex
  chmod -R 777 /usr/bin/vortex

  if [ ! -d "/etc/systemd/system" ]; then
    mkdir /etc/systemd/system
  fi

  if [ ! -f "/etc/systemd/system/vortex.service" ]; then
    cat >/etc/systemd/system/vortex.service <<EOF
[Unit]
Description=vortex

[Service]
Type=simple
User=root
Restart=always
RestartSec=5
DynamicUser=true
ExecStart=/usr/bin/vortex agent start -C /etc/vortex/config.json
ReadWritePaths=/etc/vortex /etc/gost
[Install]
WantedBy=multi-user.target
EOF
  fi

  echo ">>> vortex service installed successfully"
}

update_vortex_config() {
  local dir
  dir=$(dirname "$(readlink -f "$0")")

  if [ ! -d "/etc/vortex" ]; then
    mkdir /etc/vortex
  fi

  cat >/etc/vortex/config.json <<EOF
{
    "id": "$vortex_agent_id",
    "key": "$vortex_agent_key",
    "server": "$vortex_server",
    "dir": "$dir",
    "logLevel": "info",
    "logDest": "remote"
}
EOF
  echo ">>> vortex config updated successfully"
}

install_gost() {
  if [ -f "/usr/bin/gost" ]; then
    local installed_version
    installed_version=$(gost -V | awk '{print $2}')
    echo ">>> gost Installed version: $installed_version"
    echo ">>> gost Required version: $gost_version"
    if [ "$installed_version" = "v$gost_version" ]; then
      echo ">>> gost already installed"
      return
    fi
  fi
  local url="https://github.com/go-gost/gost/releases/download/v$gost_version/gost_${gost_version}_linux_$arch.tar.gz"
  download "$url" gost
  chmod -R 777 /usr/bin/gost

  if [ ! -d "/etc/gost" ]; then
    mkdir /etc/gost
  fi

  if [ ! -f "/etc/gost/config.json" ]; then
    cat >/etc/gost/config.json <<EOF
{
    "Debug": false
}
EOF
  fi

  if [ ! -d "/etc/systemd/system" ]; then
    mkdir /etc/systemd/system
  fi

  if [ ! -f "/etc/systemd/system/gost.service" ]; then
    cat >/etc/systemd/system/gost.service <<EOF
[Unit]
Description=gost
After=network-online.target
Wants=network-online.target systemd-networkd-wait-online.service

[Service]
Type=simple
User=root
Restart=always
RestartSec=5
DynamicUser=true
ExecStart=/usr/bin/gost -C /etc/gost/config.json
ReadWritePaths=/etc/gost

[Install]
WantedBy=multi-user.target
EOF
  fi

  echo ">>> gost service installed successfully"
}

get_arch
check_root
download_script "https://raw.githubusercontent.com/jarvis2f/vortex-agent/main/scripts/iptables.sh"
/bin/bash ./iptables.sh

if [ $# -eq 0 ]; then
  echo ">>> Invalid parameter"
  exit 1
fi

decoded_params=$(echo -n "$1" | base64 -d)
if [ -z "$decoded_params" ]; then
  echo ">>> Invalid parameter"
  exit 1
fi

IFS='|' read -ra params <<<"$decoded_params"
if [ "${#params[@]}" -ne 3 ]; then
  echo ">>> Incorrect number of parameters"
  exit 1
fi

vortex_agent_id="${params[0]}"
vortex_agent_key="${params[1]}"
vortex_server="${params[2]}"

install_gost
install_vortex_agent
update_vortex_config
service vortex start
