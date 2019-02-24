# gotmuch

## Status: barely working

This program is useful for me, but only in a limited fashion.  It can download
mail from GMail in a way that `notmuch` can index it, but there is no label/tag
synchronization in either direction, and no synchronization of deleted mail.

## Functionality and Goals

1. Synchronize GMail messages to local disk, where they can be indexed with
   `notmuch` (see http://notmuchmail.org).
2. Synchronize GMail labels with `notmuch` tags, in both directions.
3. Learn a bit about programming in Go.  ;-)

This allows the user the best of both worlds: ubiquitous access to their email
with the standard GMail interfaces, but access to the extremely flexible
tagging, filtering, and email processing capabilities of `notmuch`.

## Similar Programs

Gmailieer: https://github.com/gauteh/gmailieer -- addresses the same problem,
and it is more complete than `gotmuch`.  Written in Python.

muchsync: http://www.muchsync.org/ -- addrsses a different problem of
synchronizing multiple `notmuch` mail stores.

# Disclaimer

This is not an official Google product.
