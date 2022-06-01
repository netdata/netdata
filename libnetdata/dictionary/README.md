<!--
custom_edit_url: https://github.com/netdata/netdata/edit/master/libnetdata/dictionary/README.md
-->

# Dictionaries

Netdata dictionaries associate a `name` with a `value`:

- A `name` can be any string.
- A `value` can be anything.

Such a pair of a `name` and a `value` consists of an `item` or an `entry` in the dictionary.

Dictionaries provide an interface to:

- **Add** an item to the dictionary
- **Get** an item from the dictionary (provided its `name`)
- **Delete** an item from the dictionary (provided its `name`)
- **Traverse** the list of items in the dictionary

Dictionaries are **ordered**, meaning that the order they have been added is preserved while traversing them. The caller may reverse this order by passing the flag `DICTIONARY_FLAG_ADD_IN_FRONT` when creating the dictionary.

Dictionaries guarantee **uniqueness** of all items added to them, meaning that only one item with a given name can exist in the dictionary at any given time.

Dictionaries are extremely fast in all operations. They are indexing the keys with `JudyHS` and they utilize a double-linked-list for the traversal operations. Deletion is the most expensive operation, usually somewhat slower than insertion.

## Memory management

Dictionaries come with 2 memory management options:

- **Clone** (copy) the name and/or the value to memory allocated by the dictionary.
- **Link** the name and/or the value, without allocating any memory about them.

In **clone** mode, the dictionary guarantees that all operations on the dictionary items will automatically take care of the memory used by the name and/or the value. In case the value is an object needs to have user allocated memory, two callback functions can be registered:

   1.`dictionary_register_insert_callback()` that will be called just after the insertion of an item to the dictionary (but while the dictionary is write-locked - if locking is enabled).
   2. `dictionary_register_delete_callback()` that will be called just prior to the deletion of an item from the dictionary (but while the dictionary is write-locked - if locking is enabled).

In **link** mode, the name and/or the value are just linked to the dictionary item, and it is the user's responsibility to free the memory used after an item is deleted from the dictionary.

By default, **clone** mode is used for both the name and the value.

To use **link** mode for names, add `DICTIONARY_FLAG_NAME_LINK_DONT_CLONE` to the flags when creating the dictionary.

To use **link** mode for values, add `DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE` to the flags when creating the dictionary.

## Locks

The dictionary allows both **single-threaded** operation (no locks - faster) and **multi-threaded** operation utilizing a read-write lock.

The default is **multi-threaded**. To enable **single-threaded** add `DICTIONARY_FLAG_SINGLE_THREADED` to the flags when creating the dictionary.

## Hash table operations

The dictionary supports the following operations supported by the hash table:

- `dictionary_set()` to add an item to the dictionary, or change its value.
- `dictionary_get()` to get an item from the dictionary.
- `dictionary_del()` to delete an item from the dictionary.

## Creation and destruction

Use `dictionary_create()` to create a dictionary.

Use `dictionary_destroy()` to destroy a dictionary. When destroyed, a dictionary frees all the memory it has allocated on its own. The exception is the registration of a deletion callback function that can be called on deletion of an item, which may free additional resources. 

### dictionary_set()

This call is used to:

- **add** an item to the dictionary.
- **reset** the value of an existing item in the dictionary.

If **resetting** is not desired, add `DICTIONARY_FLAG_DONT_OVERWRITE_VALUE` to the flags when creating the dictionary. In this case, `dictionary_set()` will return the value of the original item found in the dictionary instead of resetting it and the value passed to the call will be ignored.

For **multi-threaded** operation, the `dictionary_set()` calls get an exclusive write lock on the dictionary.

The format is:

```c
value = dictionary_set(dict, name, value, value_len);
```

Where:

* `dict` is a pointer to the dictionary previously created.
* `name` is a pointer to a string to be used as the key of this item. The name must not be `NULL` and must not be an empty string `""`.
* `value` is a pointer to the value associated with this item. In **clone** mode, if `value` is `NULL`, a new memory allocation will be made of `value_len` size and will be initialized to zero.
* `value_len` is the size of the `value` data. If `value_len` is zero, no allocation will be done and the dictionary item will permanently have the `NULL` value.

> **IMPORTANT**<br/>There is also an **unsafe** version (without locks) of this call. This is to be used when traversing the dictionary. It should never be called without an active lock on the dictionary, which can only be acquired while traversing.

### dictionary_get()

This call is used to get the value of an item, given its name. It utilizes the JudyHS hash table for making the lookup.

For **multi-threaded** operation, the `dictionary_get()` call gets a shared read lock on the dictionary.

The format is:

```c
value = dictionary_get(dict, name);
```

Where:

* `dict` is a pointer to the dictionary previously created.
* `name` is a pointer to a string to be used as the key of this item. The name must not be `NULL` and must not be an empty string `""`.

> **IMPORTANT**<br/>There is also an **unsafe** version (without locks) of this call. This is to be used when traversing the dictionary. It should never be called without an active lock on the dictionary, which can only be acquired while traversing.

### dictionary_del()

This call is used to delete an item from the dictionary, given its name.

If there is a delete callback registered to the dictionary (`dictionary_register_delete_callback()`), it is called prior to the actual deletion of the item. 

For **multi-threaded** operation, the `dictionary_del()` calls get an exclusive write lock on the dictionary.

The format is:

```c
value = dictionary_del(dict, name);
```

Where:

* `dict` is a pointer to the dictionary previously created.
* `name` is a pointer to a string to be used as the key of this item. The name must not be `NULL` and must not be an empty string `""`.

> **IMPORTANT**<br/>There is also an **unsafe** version (without locks) of this call. This is to be used when traversing the dictionary, to delete the current item. It should never be called without an active lock on the dictionary, which can only be acquired while traversing.

## Traversal

Dictionaries offer 2 ways to traverse the entire dictionary:

- **walkthrough**, implemented by setting a callback function to be called for every item.
- **foreach**, a way to traverse the dictionary with a for-next loop.

Both of these methods are available in **read** or **write** mode. In **read** mode only lookups are allowed to the dictionary. In **write** both lookups but also deletion of the currently working item is also allowed.

While traversing the dictionary with any of these methods, all calls to the dictionary have to use the `_unsafe` versions of the function calls, otherwise deadlock may arise.

> **IMPORTANT**<br/>The dictionary itself does not check to ensure that a user is actually using the right lock mode (read or write) while traversing the dictionary for each of the unsafe calls.

### walkthrough (callback)

There are 2 calls:

- `dictionary_walkthrough_read()` that acquires a shared read lock, and it calls a callback function for every item of the dictionary. The callback function may use the unsafe versions of the `dictionary_get()` calls to lookup other items in the dictionary, but it should not add or remove item from the dictionary.
- `dictionary_walkthrough_write()` that acquires an exclusive write lock, and it calls a callback function for every item of the dictionary. This is to be used when items need to be added to the dictionary, or when the current item may need to be deleted. If the callback function deletes any other items, the behavior may be undefined (actually, the item next to the one currently working should not be deleted - a pointer to it is held by the traversal function to move on traversing the dictionary).

The items are traversed in the same order they have been added to the dictionary (or the reverse order if the flag `DICTIONARY_FLAG_ADD_IN_FRONT` is set during dictionary creation).

The callback function returns an `int`. If this value is negative, traversal of the dictionary is stopped immediately and the negative value is returned to the caller. If the returned value of all callbacks is zero or positive, the walkthrough functions return the sum of the return values of all callbacks. So, if you are just interested to know how many items fall into some condition, write a callback function that returns 1 when the item satisfies that condition and 0 when it does not and the walkthrough function will return how many tested positive.

### foreach (for-next loop)

The following is a snippet of such a loop:

```c
MY_ITEM *item;
dfe_start_read(dict, item) {
   printf("hey, I got an item named '%s' with value ptr %08X", item_name, item);
}
dfe_done(item);
```

The `item` parameter gives the name of the pointer to be used while iterating the items. Any name is accepted.

The `item_name` is a variable that is automatically created, by concatenating whatever is given as `item` and `_name`. So, if you call `dfe_start_read(dict, myvar)`, the name will be `myvar_name`.

Both `dfe_start_read(dict, item)` and `dfe_done(item)` are together inside a `do { ... } while(0)` loop, so that the following will work:

```c
MY_ITEM *item;

if(x = 1)
    // do {
    dfe_start_read(dict, item)
        printf("hey, I got an item named '%s' with value ptr %08X", item_name, item);
    dfe_done(item);
    // } while(0);
else
    something else;
```

In the above, the `if(x)` condition will work as expected. It will do the foreach loop when x is 1, otherwise it will run `something else`.

There are 2 versions of `dfe_start`:

- `dfe_start_read()` that acquires a shared read lock to the dictionary.
- `dfe_start_write()` that acquires an exclusive write lock to the dictionary.

While in the loop, depending on the read or write versions of `dfe_start`, the caller may lookup or manipulate the dictionary. The rules are the same with the walkthrough callback functions.

PS: DFE is Dictionary For Each.

## special multi-threaded lockless case

Since the dictionary uses a hash table and a double linked list, if the contract between 2 threads is for one to use the hash table functions only (`set`, `get` - but no `del`) and the other to use the traversal ones only, the dictionary allows concurrent use without locks.

This is currently used in statsd:

- the data collection thread uses only `get` and `set`. It never uses `del`. New items are added at the front of the linked list (`DICTIONARY_FLAG_ADD_IN_FRONT`).
- the flushing thread is only traversing the dictionary up to the point it last traversed it (it uses a flag for that to know where it stopped last time). It never uses `get`, `set` or `del`.
