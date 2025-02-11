// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_SENTRY_H
#define ND_SENTRY_H

void nd_sentry_init(void);
void nd_sentry_fini(void);

void nd_sentry_set_user(const char *guid);
void nd_sentry_add_breadcrumb(const char *message);

#endif /* ND_SENTRY_H */
