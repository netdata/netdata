#ifndef NETDATA_DICTIONARY_H
#define NETDATA_DICTIONARY_H 1

/**
 * @file dictionary.h
 * @brief Dictionary to access data.
 *
 * A dictionary maps names to values. `name` is a string which identifies the `value`. `value` can be any data.
 *
 * A dictionary is able to maintain statistics of then number of etries and insert, delte and get operations.
 */

/** Dictionary statistics */
struct dictionary_stats {
    unsigned long long inserts;  ///< Number of inserts completed.
    unsigned long long deletes;  ///< Number of deletes completed.
    unsigned long long searches; ///< Number of searches made.
    unsigned long long entries;  ///< Number of entries.
};

/** Dictionary entry */
typedef struct name_value {
    // this has to be first!
    avl avl;                ///< the index

    uint32_t hash;          ///< a simple hash to speed up searching
                            ///< we first compare hashes, and only if the hashes are equal we do string comparisons

    char *name;  ///< name
    void *value; ///< value
} NAME_VALUE;

/** Dictionary */
typedef struct dictionary {
    avl_tree values_index; ///< tree of values

    uint8_t flags; ///< DICTIONARY_FLAG_*

    struct dictionary_stats *stats; ///< Statistics of this dictionary. This may be NULL.
    pthread_rwlock_t *rwlock;       ///< Lock for synchronizing access to this. This may be NULL.
} DICTIONARY;

#define DICTIONARY_FLAG_DEFAULT                 0x00000000 ///< No specific meaning.
#define DICTIONARY_FLAG_SINGLE_THREADED         0x00000001 ///< If set, do not synchronize access to the dictionary. 
                                                           ///< This can lead to crashes if multiple threads are 
                                                           ///< adding, removing and reading the tree concurrently.
#define DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE   0x00000002 ///< Only link the value. Do not clone it.
#define DICTIONARY_FLAG_NAME_LINK_DONT_CLONE    0x00000004 ///< Only link the name. Do not clone it.
#define DICTIONARY_FLAG_WITH_STATISTICS         0x00000008 ///< Maintain statistics for this dictionary.

/**
 * Create an empty dictionary.
 *
 * Options can be set with `flags`.
 *
 * @param flags DICTIONARY_FLAG_*
 * @return a dictionary.
 */
extern DICTIONARY *dictionary_create(uint8_t flags);
/**
 * Free dictionary.
 *
 * Free a dictionary allocated with `dictionary_create()`.
 *
 * @param dict The dictionary.
 */
extern void dictionary_destroy(DICTIONARY *dict);
/**
 * Add a name value pair to a dictionary.
 *
 * If key is present replace the value.
 *
 * @param dict The dictionary.
 * @param name to set
 * @param value to set
 * @param value_len Size of `value` in bytes.
 * @return the inserted value
 */
extern void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len);
/**
 * Get the value of name in a dictionary.
 *
 * @param dict The dictionary.
 * @param name to find
 * @return value or NULL
 */
extern void *dictionary_get(DICTIONARY *dict, const char *name);
/**
 * Delete a name value pair in a dictionary.
 *
 * @param dict The dictionary.
 * @name to delete.
 * @return 0 on succes, -1 on error.
 */
extern int dictionary_del(DICTIONARY *dict, const char *name);

/**
 * Get all dictionary items.
 *
 * This calls `callback(entry, d)` on each entry. 
 * The `value` of each entry is passed to entry. `data` is passed to `d`.
 * If `callback()` returns 0 or less `dictionary_get_all()` stops traversing
 * and returns the same value.
 *
 * If each callback returns `> 0` the number `callback()` was called is returned.
 *
 * __Example__ `callback()`
 * ```{.c}
 * static int print_entry(void *entry, void *anything) {
 *   char *value = entry;
 *   return printf("%s\n", value);
 * }
 * ```
 * This assumes `value` of each entry is a string and prints it to `fstdout`.
 *
 * @param dict The dictionary.
 * @param callback called on each entry
 * @param data passed into callback
 * @return number of items or return code of `callback()` if `<= 0`
 */
extern int dictionary_get_all(DICTIONARY *dict, int (*callback)(void *entry, void *d), void *data);

#endif /* NETDATA_DICTIONARY_H */
