The voucher_db file in this directory is a copy of a manufacturing
server database that contains two ownership vouchers. These vouchers
were created by running the DI protocol using the go-fdo-client
against the manufacturing server.

The database contains two vouchers with the following GUIDs:

- "fe851cc3a2fe08166b364b191cfbb5d0"
- "6127c9733b12651a340af022faaca9f3"

This database is used by the vouchers_test.go tests. The test makes a
working copy of this database on startup so the test can modify the
working copy without affecting the original.
