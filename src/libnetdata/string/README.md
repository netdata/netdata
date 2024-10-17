# STRING

STRING provides a way to allocate and free text strings, while de-duplicating them.

It can be used similarly to libc string functions:

 - `strdup()` and `strdupz()` become `string_strdupz()`.
 - `strlen()` becomes `string_strlen()` (and it does not walkthrough the bytes of the string).
 - `free()` and `freez()` become `string_freez()`.

There is also a special `string_dup()` function that increases the reference counter of a STRING, avoiding the
index lookup to find it.

Once there is a `STRING *`, the actual `const char *` can be accessed with `string2str()`.

All STRING should be constant. Changing the contents of a `const char *` that has been acquired by `string2str()` should never happen. 
