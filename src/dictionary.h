#ifndef NETDATA_DICTIONARY_H
#define NETDATA_DICTIONARY_H 1

/** Statistics of a dictionary */
struct dictionary_stats {
    unsigned long long inserts;  ///< Number of inserts completed.
    unsigned long long deletes;  ///< Number of deletes completed.
    unsigned long long searches; ///< Number of searches made.
    unsigned long long entries;  ///< Number of entries.
};

/** Name value pair */
typedef struct name_value {
    // this has to be first!
    avl avl;                ///< the index

    uint32_t hash;          ///< a simple hash to speed up searching
                            ///< we first compare hashes, and only if the hashes are equal we do string comparisons

    char *name;  ///< name
    void *value; ///< value
} NAME_VALUE;

/** A dictionary */
typedef struct dictionary {
    avl_tree values_index; ///< tree of values

    uint8_t flags; ///< DICTIONARY_FLAG_*

    struct dictionary_stats *stats; ///< statistics of this dictionary
    pthread_rwlock_t *rwlock;       ///< lock for synchronizing access to this
} DICTIONARY;

#define DICTIONARY_FLAG_DEFAULT                 0x00000000
#define DICTIONARY_FLAG_SINGLE_THREADED         0x00000001
#define DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE   0x00000002
#define DICTIONARY_FLAG_NAME_LINK_DONT_CLONE    0x00000004
#define DICTIONARY_FLAG_WITH_STATISTICS         0x00000008

extern DICTIONARY *dictionary_create(uint8_t flags);
extern void dictionary_destroy(DICTIONARY *dict);
extern void *dictionary_set(DICTIONARY *dict, const char *name, void *value, size_t value_len);
extern void *dictionary_get(DICTIONARY *dict, const char *name);
extern int dictionary_del(DICTIONARY *dict, const char *name);

extern int dictionary_get_all(DICTIONARY *dict, int (*callback)(void *entry, void *d), void *data);

#endif /* NETDATA_DICTIONARY_H */
