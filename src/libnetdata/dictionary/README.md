<!--
custom_edit_url: https://github.com/netdata/netdata/edit/master/src/libnetdata/dictionary/README.md
sidebar_label: "Dictionaries"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Developers/libnetdata"
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

Dictionaries are **ordered**, meaning that the order they have been added, is preserved while traversing them. The caller may reverse this order by passing the flag `DICT_OPTION_ADD_IN_FRONT` when creating the dictionary.

Dictionaries guarantee **uniqueness** of all items added to them, meaning that only one item with a given `name` can exist in the dictionary at any given time.

Dictionaries are extremely fast in all operations. They are indexing the keys with `JudyHS` and they utilize a double-linked-list for the traversal operations. Deletion is the most expensive operation, usually somewhat slower than insertion.

## Memory management

Dictionaries come with 2 memory management options:

- **Clone** (copy) the `name` and/or the `value` to memory allocated by the dictionary.
- **Link** the `name` and/or the `value`, without allocating any memory about them.

In **clone** mode, the dictionary guarantees that all operations on the dictionary items, will automatically take care of the memory used by the `name` and/or the `value`. In case the `value` is an object that needs to have user allocated memory, the following callback functions can be registered:

1. `dictionary_register_insert_callback()` that can be called just after the insertion of an item to the dictionary, or after the replacement of the value of a dictionary item.
2. `dictionary_register_delete_callback()` that will be called just prior to the deletion of an item from the dictionary, or prior to the replacement of the value of a dictionary item.
3. `dictionary_register_conflict_callback()` that will be called when `DICT_OPTION_DONT_OVERWRITE_VALUE` is set, and another `value` is attempted to be inserted for the same key.
4. `dictionary_register_react_callback()` that will be called after the the `insert` and the `conflict` callbacks. The `conflict` callback is called while the dictionary hash table is available for other threads.

In **link** mode, the `name` and/or the `value` are just linked to the dictionary item, and it is the user's responsibility to free the memory they use after an item is deleted from the dictionary or when the dictionary is destroyed.

By default, **clone** mode is used for both the name and the value.

To use **link** mode for names, add `DICT_OPTION_NAME_LINK_DONT_CLONE` to the flags when creating the dictionary.

To use **link** mode for values, add `DICT_OPTION_VALUE_LINK_DONT_CLONE` to the flags when creating the dictionary.

## Locks

The dictionary allows both **single-threaded** operation (no locks - faster) and **multi-threaded** operation utilizing a read-write lock.

The default is **multi-threaded**. To enable **single-threaded** add `DICT_OPTION_SINGLE_THREADED` to the flags when creating the dictionary.

When in **multi-threaded** mode, the dictionaries have 2 independent R/W locks. One for the linked list and one for the hash table (index). An insertion and a deletion will acquire both independently (one after another) for as long as they are needed, but a traversal may hold the the linked list for longer durations. The hash table (index) lock may be acquired while the linked list is acquired, but not the other way around (and the way the code is structured, it is not technically possible to hold and index lock and then lock the linked list one).

These locks are R/W locks. They allow multiple readers, but only one writer.

Unlike POSIX standards, the linked-list lock, allows one writer to lock it multiple times. This has been implemented in such a way, so that a traversal to the items of the dictionary in write-lock mode, allows the writing thread to call `dictionary_set()` or `dictionary_del()`, which alter the dictionary index and the linked list. Especially for the deletion of the currently working item, the dictionary support delayed removal, so it will remove it from the index immediately and mark it as deleted, so that it can be added to the dictionary again with a different value and the traversal will still proceed from the point it was. 

## Hash table operations

The dictionary supports the following operations supported by the hash table:

- `dictionary_set()` to add an item to the dictionary, or change its value.
- `dictionary_get()` and `dictionary_get_and_acquire_item()` to get an item from the dictionary.
- `dictionary_del()` to delete an item from the dictionary.

For all the calls, there are also `*_advanced()` versions of them, that support more parameters. Check the header file for more information about them.

## Creation and destruction

Use `dictionary_create()` to create a dictionary.

Use `dictionary_destroy()` to destroy a dictionary. When destroyed, a dictionary frees all the memory it has allocated on its own. This can be complemented by the registration of a deletion callback function that can be called upon deletion of each item in the dictionary, which may free additional resources linked to it. 

### dictionary_set()

This call is used to:

- **add** an item to the dictionary.
- **reset** the value of an existing item in the dictionary.

If **resetting** is not desired, add `DICT_OPTION_DONT_OVERWRITE_VALUE` to the flags when creating the dictionary. In this case, `dictionary_set()` will return the value of the original item found in the dictionary instead of resetting it and the value passed to the call will be ignored. Optionally a conflict callback function can be registered, to manipulate (probably merge or extend) the original value, based on the new value attempted to be added to the dictionary.

The format is:

```c
value = dictionary_set(dict, name, value, value_len);
```

Where:

* `dict` is a pointer to the dictionary previously created.
* `name` is a pointer to a string to be used as the key of this item. The name must not be `NULL` and must not be an empty string `""`.
* `value` is a pointer to the value associated with this item. In **clone** mode, if `value` is `NULL`, a new memory allocation will be made of `value_len` size and will be initialized to zero.
* `value_len` is the size of the `value` data in bytes. If `value_len` is zero, no allocation will be done and the dictionary item will permanently have the `NULL` value.

### dictionary_get()

This call is used to get the `value` of an item, given its `name`. It utilizes the hash table (index) for making the lookup.

For **multi-threaded** operation, the `dictionary_get()` call gets a shared read lock on the index lock (multiple readers are allowed). The linked-list lock is not used.

In clone mode, the value returned is not guaranteed to be valid, as any other thread may delete the item from the dictionary at any time. To ensure the value will be available, use `dictionary_get_and_acquire_item()`, which uses a reference counter to defer deletes until the item is released with `dictionary_acquired_item_release()`.

The format is:

```c
value = dictionary_get(dict, name);
```

Where:

* `dict` is a pointer to the dictionary previously created.
* `name` is a pointer to a string to be used as the key of this item. The name must not be `NULL` and must not be an empty string `""`.

### dictionary_del()

This call is used to delete an item from the dictionary, given its name.

If there is a deletion callback registered to the dictionary (`dictionary_register_delete_callback()`), it is called prior to the actual deletion of the item. 

The format is:

```c
value = dictionary_del(dict, name);
```

Where:

* `dict` is a pointer to the dictionary previously created.
* `name` is a pointer to a string to be used as the key of this item. The name must not be `NULL` and must not be an empty string `""`.

### dictionary_get_and_acquire_item()

This call can be used to search and acquire a dictionary item, while ensuring that it will be available for use, until `dictionary_acquired_item_release()` is called.

This call **does not return the value** of the dictionary item. It returns an internal pointer to a structure that maintains the reference counter used to protect the actual value. To get the value of the item (the same value as returned by `dictionary_get()`), the function `dictionary_acquired_item_value()` has to be called.

Example:

```c
// create the dictionary
DICTIONARY *dict = dictionary_create(DICT_OPTION_NONE);

// add an item to it
dictionary_set(dict, "name", "value", 6);

// find the item we added and acquire it
const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(dict, "name");

// extract its value
char *value = (char *)dictionary_acquired_item_value(dict, item);

// now value points to the string "value"
printf("I got value = '%s'\n", value);

// release the item, so that it can deleted
dictionary_acquired_item_release(dict, item);

// destroy the dictionary
dictionary_destroy(dict);
```

When items are acquired, a reference counter is maintained to keep track of how many users exist for it. If an item with a non-zero number of users is deleted, it is removed from the index, it can be added again to the index (without conflict), and although it exists in the linked-list, it is not offered during traversal. Garbage collection to actually delete the item happens every time another item is added or removed from the linked-list and items are deleted only if no users are using them.

If any item is still acquired when the dictionary is destroyed, the destruction of the dictionary is also deferred until all the acquired items are released. When the dictionary is destroyed like that, all operations on the dictionary fail (traversals do not traverse, insertions do not insert, deletions do not delete, searches do not find any items, etc). Once the last item in the dictionary is released, the dictionary is automatically destroyed too.

## Traversal

Dictionaries offer 3 ways to traverse the entire dictionary:

- **walkthrough**, implemented by setting a callback function to be called for every item.
- **sorted walkthrough**, which first sorts the dictionary and then call a callback function for every item.
- **foreach**, a way to traverse the dictionary with a for-next loop.

All these methods are available in **read**, **write**, or **reentrant** mode. In **read** mode only lookups are allowed to the dictionary. In **write** lookups but also insertions and deletions are allowed, and in **reentrant** mode the dictionary is unlocked outside dictionary code.

### walkthrough (callback)

There are 4 calls:

- `dictionary_walkthrough_read()` and `dictionary_sorted_walkthrough_read()` acquire a shared read lock on the linked-list, and they call a callback function for every item of the dictionary.
- `dictionary_walkthrough_write()` and `dictionary_sorted_walkthrough_write()` acquire a write lock on the linked-list, and they call a callback function for every item of the dictionary. This is to be used when items need to be added to or removed from the dictionary. The `write` versions can be used to delete any or all the items from the dictionary, including the currently working one. For the `sorted` version, all items in the dictionary maintain a reference counter, so all deletions are deferred until the sorted walkthrough finishes. 

The non sorted versions traverse the items in the same order they have been added to the dictionary (or the reverse order if the flag `DICT_OPTION_ADD_IN_FRONT` is set during dictionary creation). The sorted versions sort alphabetically the items based on their name, and then they traverse them in the sorted order.

The callback function returns an `int`. If this value is negative, traversal of the dictionary is stopped immediately and the negative value is returned to the caller. If the returned value of all callback calls is zero or positive, the walkthrough functions return the sum of the return values of all callbacks. So, if you are just interested to know how many items fall into some condition, write a callback function that returns 1 when the item satisfies that condition and 0 when it does not and the walkthrough function will return how many tested positive.

### foreach (for-next loop)

The following is a snippet of such a loop:

```c
MY_STRUCTURE *x;
dfe_start_read(dict, x) {
   printf("hey, I got an item named '%s' with value ptr %08X", x_dfe.name, x);
}
dfe_done(x);
```

The `x` parameter gives the name of the pointer to be used while iterating the items. Any name is accepted. `x` points to the `value` of the item in the dictionary.

The `x_dfe.name` is a variable that is automatically created, by concatenating whatever is given as `x` and `_dfe`. It is an object and it has a few members, including `x_dfe.counter` that counts the iterations made so far, `x_dfe.item` that provides the acquired item from the dictionary and which can be used to pass it over for further processing, etc. Check the header file for more info. So, if you call `dfe_start_read(dict, myvar)`, the name will be `myvar_dfe`.

Both `dfe_start_read(dict, item)` and `dfe_done(item)` are together inside a `do { ... } while(0)` loop, so that the following will work:

```c
MY_ITEM *item;

if(a = 1)
    // do {
    dfe_start_read(dict, x)
        printf("hey, I got an item named '%s' with value ptr %08X", x_dfe.name, x);
    dfe_done(x);
    // } while(0);
else
    something else;
```

In the above, the `if(a == 1)` condition will work as expected. It will do the foreach loop when a is 1, otherwise it will run `something else`.

There are 2 versions of `dfe_start`:

- `dfe_start_read()` that acquires a shared read linked-list lock to the dictionary.
- `dfe_start_write()` that acquires an exclusive write linked-list lock to the dictionary.

While in the loop, depending on the read or write versions of `dfe_start`, the caller may lookup or manipulate the dictionary. The rules are the same with the unsorted walkthrough callback functions.

PS: DFE is Dictionary For Each.
