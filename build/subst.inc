.in:
	if sed \
		-e 's#[@]localstatedir_POST@#$(localstatedir)#g' \
		-e 's#[@]sbindir_POST@#$(sbindir)#g' \
		$< > $@.tmp; then \
		mv "$@.tmp" "$@"; \
	else \
		rm -f "$@.tmp"; \
		false; \
	fi
