// SPDX-License-Identifier: GPL-3.0-or-later

/*
Package web contains HTTP request and client configurations.
HTTPConfig structure embeds both of them, and it's the only structure that intended to be used as part of a module's configuration.
Every module that uses HTTP requests to collect metrics should use it.
It allows to have same set of user configurable options across all modules.
*/
package web
