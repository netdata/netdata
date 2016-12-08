%global debug_package %{nil}

# This is temporary and should eventually be resolved. This bypasses
# the default rhel __os_install_post which throws a python compile
# error.
%global __os_install_post %{nil}

# Conditional build
%if 0%{?fedora}
%bcond_without nfacct
%endif
%if 0%{?fedora} >= 17 || 0%{?rhel} >= 7
%bcond_without systemd
%endif

Name:    netdata
Version: 1.4.1
Release: 1%{?dist}
Summary: Real-time performance monitoring, done right
License: GPLv3+
Group:   Applications/System
URL:     http://github.com/baoboa/netdata
Source0: %{url}/releases/download/v%{version}/%{name}-%{version}.tar.gz
Source1: %{name}.conf
Source2: %{name}.tmpfiles
Source3: %{name}.logrotate
Source4: %{name}.init

BuildRequires: libtool
BuildRequires: pkgconfig(uuid)
BuildRequires: pkgconfig(zlib)
%if %{with nfacct}
BuildRequires: pkgconfig(libmnl)
BuildRequires: pkgconfig(libnetfilter_acct)
%endif
%if %{with systemd}
BuildRequires: systemd
Requires(post): systemd
Requires(preun): systemd
Requires(postun): systemd
%else
Requires(post): chkconfig
Requires(preun): chkconfig, initscripts
Requires(postun): initscripts
%endif

%description
netdata is the fastest way to visualize metrics. It is a resource
efficient, highly optimized system for collecting and visualizing any
type of realtime timeseries data, from CPU usage, disk activity, SQL
queries, API calls, web site visitors, etc.

netdata tries to visualize the truth of now, in its greatest detail,
so that you can get insights of what is happening now and what just
happened, on your systems and applications.

%prep
%setup -q

%build
autoreconf -ivf
%configure \
    --with-zlib \
    --with-math \
    %{?with_nfacct:--enable-plugin-nfacct} \
    --with-user=%{name}
%{make_build}

%install
%{make_install}
find %{buildroot} -name .keep -delete

# Unit file
%if %{with systemd}
install -Dp -m0644 system/%{name}.service %{buildroot}%{_unitdir}/%{name}.service
install -Dp -m0644 %{SOURCE2} %{buildroot}%{_tmpfilesdir}/%{name}.conf
%else
# Init script
install -Dp -m0755 %{SOURCE4} %{buildroot}%{_initrddir}/%{name}
%endif
install -Dp -m0644 %{SOURCE3} %{buildroot}%{_sysconfdir}/logrotate.d/%{name}
install %{SOURCE1} %{buildroot}%{_sysconfdir}/%{name}/
%{__mkdir_p} %{buildroot}%{_localstatedir}/lib/%{name}

%pre
getent group %{name} > /dev/null || groupadd -r %{name}
getent passwd %{name} > /dev/null || useradd -r -g %{name} \
    -c "NetData User" -s /sbin/nologin -d %{_localstatedir}/lib/%{name} %{name}

%post
%if %{with systemd}
%systemd_post %{name}.service
%else
if [ $1 -eq 1 ]; then
    /sbin/chkconfig --add %{name}
fi
%endif

%preun
%if %{with systemd}
%systemd_preun %{name}.service
%else
if [ $1 -eq 0 ]; then
    /sbin/service %{name} stop &>/dev/null ||:
    /sbin/chkconfig --del %{name}
fi
%endif

%postun
%if %{with systemd}
%systemd_postun_with_restart %{name}.service
%else
if [ $1 -eq 1 ]; then
    /sbin/service %{name} restart &>/dev/null ||:
fi
%endif

%files
%defattr(-,root,root,-)
%doc README.md
%license COPYING

%dir %{_sysconfdir}/%{name}
%config(noreplace) %{_sysconfdir}/%{name}/*.conf
%config(noreplace) %{_sysconfdir}/%{name}/health.d/*.conf
%config(noreplace) %{_sysconfdir}/%{name}/python.d/*.conf
%config(noreplace) %{_sysconfdir}/logrotate.d/%{name}

%{_sbindir}/%{name}
%{_libexecdir}/%{name}

%attr(-,%{name},%{name}) %dir %{_localstatedir}/cache/%{name}
%attr(-,%{name},%{name}) %dir %{_localstatedir}/log/%{name}
%attr(-,%{name},%{name}) %dir %{_localstatedir}/lib/%{name}

%dir %{_datadir}/%{name}
%attr(-,root,%{name}) %{_datadir}/%{name}/web

%if %{with systemd}
%{_unitdir}/%{name}.service
%{_tmpfilesdir}/%{name}.conf
%else
%attr(0755,root,root) %{_initrddir}/%{name}
%endif

%changelog
* Fri Dec 09 2016 baoboadev <baoboa@fedoraproject.org> 1.4.1-1
- switch archive to tgz (baoboa@fedoraproject.org)
- Update README.md (baoboadev@gmail.com)
- Update README.md (baoboadev@gmail.com)
- ready to test copr (baobabdev@gmail.com)

* Fri Dec 09 2016 baoboadev <baoboa@fedoraproject.org>
- switch archive to tgz (baoboa@fedoraproject.org)
- Update README.md (baoboadev@gmail.com)
- Update README.md (baoboadev@gmail.com)
- ready to test copr (baobabdev@gmail.com)

* Wed Dec 07 2016 baoboa <baobabdev@gmail.com> 1.4.0-1
- new package built with tito

* Thu Oct  6 2016 mosquito <sensor.wen@gmail.com> - 1.4.0-1
- Release 1.4.0
* Fri Sep  2 2016 mosquito <sensor.wen@gmail.com> - 1.3.0-1
- Release 1.3.0
* Sat Jun 18 2016 mosquito <sensor.wen@gmail.com> - 1.2.0-4
- Fix build error for rhel
* Sat Jun 18 2016 mosquito <sensor.wen@gmail.com> - 1.2.0-3
- Add init script, logrotate and tmpfiles config
- Create missing dir: /var/lib/netdata
* Fri Jun  3 2016 mosquito <sensor.wen@gmail.com> - 1.2.0-2
- Add autoreconf
* Fri Jun  3 2016 mosquito <sensor.wen@gmail.com> - 1.2.0-1
- Initial build
