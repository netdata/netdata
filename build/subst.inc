.in:
	if sed \
		-e 's#[@]localstatedir_POST@#$(localstatedir)#g' \
		-e 's#[@]sbindir_POST@#$(sbindir)#g' \
		-e 's#[@]pluginsdir_POST@#$(pluginsdir)#g' \
		-e 's#[@]configdir_POST@#$(configdir)#g' \
		-e 's#[@]libconfigdir_POST@#$(libconfigdir)#g' \
		-e 's#[@]pkglibexecdir_POST@#$(pkglibexecdir)#g' \
		-e 's#[@]cachedir_POST@#$(cachedir)#g' \
		-e 's#[@]registrydir_POST@#$(registrydir)#g' \
		-e 's#[@]varlibdir_POST@#$(varlibdir)#g' \
		-e 's#[@]webdir_POST@#$(webdir)#g' \
		-e 's#[@]libsysdir_POST@#$(libsysdir)#g' \
		-e 's#[@]enable_aclk_POST@#$(enable_aclk)#g' \
		-e 's#[@]enable_cloud_POST@#$(enable_cloud)#g' \
		-e 's#[@]netdata_user_POST@#$(netdata_user)#g' \
		$< > $@.tmp; then \
		mv "$@.tmp" "$@"; \
	else \
		rm -f "$@.tmp"; \
		false; \
	fi
