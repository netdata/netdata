.in:
	if sed \
		-e 's#[@]localstatedir_POST@#$(localstatedir)#g' \
		-e 's#[@]sbindir_POST@#$(sbindir)#g' \
		-e 's#[@]configdir_POST@#$(configdir)#g' \
		-e 's#[@]libconfigdir_POST@#$(libconfigdir)#g' \
		-e 's#[@]cachedir_POST@#$(cachedir)#g' \
		$< > $@.tmp; then \
		mv "$@.tmp" "$@"; \
	else \
		rm -f "$@.tmp"; \
		false; \
	fi
