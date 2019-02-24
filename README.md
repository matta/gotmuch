# gotmuch

## Status: barely working

This program is useful for me, in a limited fashion.  It can download mail from
GMail in a way that `notmuch` can index it, but there is no label/tag
synchronization in either direction, and no synchronization of deleted mail.

## Functionality and Goals

1.  Synchronize GMail with http://notmuchmail.org.
2.  Two way synchronization of tags.
3.  

This program synchronizes GMail messages and their labels to http://notmuchail.org
mail stores, where it can then be read and manipulated by `notmuch` mail readers
and scripts.

## Similar Programs

Gmailieer: https://github.com/gauteh/gmailieer -- addresses the same problem,
and it is more complete than `gotmuch`.  Written in Python.

muchsync: http://www.muchsync.org/ -- addrsses a different problem of
synchronizing multiple `notmuch` mail stores.

# Disclaimer

This is not an official Google product.

