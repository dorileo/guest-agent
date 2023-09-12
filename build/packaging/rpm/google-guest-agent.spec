# Copyright 2018 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Don't build debuginfo packages.
%define debug_package %{nil}

Name: google-guest-agent
Epoch:   1
Version: %{_version}
Release: g1%{?dist}
Summary: Google Compute Engine guest agent.
License: ASL 2.0
Url: https://cloud.google.com/compute/docs/images/guest-environment
Source0: git://%{_source_dir}

BuildArch: %{_arch}
%if ! 0%{?el6}
BuildRequires: systemd
%endif

Obsoletes: python-google-compute-engine, python3-google-compute-engine

%description
Contains the Google guest agent binary.

%prep
%autosetup

%build
SKIP_LIB_TARGETS="true" make build-all

%install
SKIP_LIB_TARGETS="true" PREFIX=%{buildroot} make install

%files
%{_docdir}/%{name}
%defattr(-,root,root,-)
/usr/share/google-guest-agent/instance_configs.cfg
%{_bindir}/google_guest_agent
%{_bindir}/google_metadata_script_runner
%{_bindir}/gce_workload_cert_refresh
%if 0%{?el6}
/etc/init/%{name}.conf
/etc/init/google-startup-scripts.conf
/etc/init/google-shutdown-scripts.conf
%else
%{_unitdir}/%{name}.service
%{_unitdir}/google-startup-scripts.service
%{_unitdir}/google-shutdown-scripts.service
%{_unitdir}/gce-workload-cert-refresh.service
%{_unitdir}/gce-workload-cert-refresh.timer
%{_presetdir}/90-%{name}.preset
%endif

%if ! 0%{?el6}
%post
if [ $1 -eq 1 ]; then
  # Initial installation

  # Install instance configs if not already present.
  if [ ! -f /etc/default/instance_configs.cfg ]; then
    cp -a /usr/share/google-guest-agent/instance_configs.cfg /etc/default/
  fi

  # Use enable instead of preset because preset is not supported in
  # chroots.
  systemctl enable google-guest-agent.service >/dev/null 2>&1 || :
  systemctl enable google-startup-scripts.service >/dev/null 2>&1 || :
  systemctl enable google-shutdown-scripts.service >/dev/null 2>&1 || :
  systemctl enable gce-workload-cert-refresh.timer >/dev/null 2>&1 || :

  if [ -d /run/systemd/system ]; then
    systemctl daemon-reload >/dev/null 2>&1 || :
    systemctl start google-guest-agent.service >/dev/null 2>&1 || :
    systemctl start gce-workload-cert-refresh.timer >/dev/null 2>&1 || :
  fi
else
  # Package upgrade
  if [ -d /run/systemd/system ]; then
    systemctl try-restart google-guest-agent.service >/dev/null 2>&1 || :
  fi
fi

%preun
if [ $1 -eq 0 ]; then
  # Package removal, not upgrade
  systemctl --no-reload disable google-guest-agent.service >/dev/null 2>&1 || :
  systemctl --no-reload disable google-startup-scripts.service >/dev/null 2>&1 || :
  systemctl --no-reload disable google-shutdown-scripts.service >/dev/null 2>&1 || :
  systemctl --no-reload disable gce-workload-cert-refresh.timer >/dev/null 2>&1 || :
  if [ -d /run/systemd/system ]; then
    systemctl stop google-guest-agent.service >/dev/null 2>&1 || :
  fi
fi

%postun
if [ $1 -eq 0 ]; then
  # Package removal, not upgrade

  if [ -f /etc/default/instance_configs.cfg ]; then
    rm /etc/default/instance_configs.cfg
  fi

  if [ -d /run/systemd/system ]; then
    systemctl daemon-reload >/dev/null 2>&1 || :
  fi
fi

%else

# EL6
%post
if [ $1 -eq 1 ]; then
  # Install instance configs if not already present.
  if [ ! -f /etc/default/instance_configs.cfg ]; then
    cp -a /usr/share/google-guest-agent/instance_configs.cfg /etc/default/
  fi

  # Initial installation
  initctl start google-guest-agent >/dev/null 2>&1 || :
else
  # Upgrade
  initctl restart google-guest-agent >/dev/null 2>&1 || :
fi

%preun
if [ $1 -eq 0 ]; then
  # Package removal, not upgrade
  initctl stop google-guest-agent >/dev/null 2>&1 || :
fi

%postun
if [ $1 -eq 0 ]; then
  # Package removal, not upgrade
  if [ -f /etc/default/instance_configs.cfg ]; then
    rm /etc/default/instance_configs.cfg
  fi
fi

%endif
