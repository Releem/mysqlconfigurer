MySQL Configure releases
---
Information about releases of the MySQLConfigurer script and online service.

MySQL Configurer 0.2.1, 2020-04-11
- Fixed rename file z_aiops_mysql.conf -> z_aiops_mysql.cnf. Issue #14 was closed
- Added rounding of variables. Issue #17 was closed.
- Added calculation max_connections. Issue #16 was closed.
- Added calculation thread_cache_size. Issue #15 was closed.
- Improve documentation. Issue #13 was closed.

MySQL Configurer 0.1.2, 2020-01-15
- Fixed "internal server error" in logging subsystem returned when the mysqltuner report contains empty parameter name. Issue #9 was closed.

MySQL Configurer 0.1.1, 2020-01-11
- Added check MySQLTuner exit code for prevent invalid requests to API. Issue #5 was closed
- Added -s option for the curl command to hide unnecessary output. Issue #2 was closed
- Fixed documentation and added check for JSON module. Issue #6 was closed
- Added old values to configuration file. Issue #4 was closed
- Fixed calculations of the innodb_buffer_pool_instances. Issue #1 was closed
- Improve advanced output for errors

MySQL Configurer 0.1.0, 2019-12-25
First release