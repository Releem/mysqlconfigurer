Releem MySQLConfigurer releases
---
Information about releases of the Releem MySQLConfigurer.

Releem MySQLConfigurer 0.9.0, 2022-03-30
- Added checks of the database server version
- Added configuration file releem.conf
- Added -u option to update Releem Agent
- Added list of variable changes. Closes #75
- Improved calculation of max_heap_table_size
- Improved calculation of tmp_table_size 
- Fixed MySQLTuner version
- Fixed metrics collection


Releem MySQLConfigurer 0.8.0, 2022-01-12
- Added support of MariaDB 10.6. Closes #82
- Added Automated subscriptions and credit card payments
- Added hostnames to servers list. Closes #77
- Improved documentation

Releem MySQLConfigurer 0.7.0, 2021-11-16
- Added Display timezone on server page. Closes #72
- Added [Documentation](https://releem.com/docs/getstarted). 
- Added Automated Installation of Releem Agent
- Fixed Cache values too high. Closes #73
- Fixed Error when no Innodb tables only MyISAM. Closes #76
- Fixed The values on the left and right are not in the same terminology. Closes #74
- Removed Removed MySQLTuner Recommendations 

Releem MySQLConfigurer 0.6.0, 2021-06-17
- Added MySQL Performance Score metric.
- Added runtime information. Closes #62
- Added Display Recommended Configuration.
- Improved documentation Installation, Usage and Tests.
- Improved calcualtion of the 'myisam_sort_buffer_size' variable.
- Improved calculation of the 'read_rnd_buffer_size' variable.
- Improved calculation of the 'sort_buffer_size' variable.
- Removed usage of "mysqltuner.pl" domain.

Releem MySQLConfigurer 0.5.0, 2021-01-30
- Added simple one step installation process. Closes #23.
- Improved documentation.
- Improved and published tests description at [releem.com](https://releem.com/blog/how-to-improve-performance-mysql57-default-configuration). Closes #31.
- Fixed problem with timeout variables. Closes #29.
- Added calculation of the 'max_allowed_packet' variable.
- Added calculation of the 'read_rnd_buffer_size' variable.
- Improved calcualtion of the 'sort_buffer_size' variable.
- Improved calculation of the 'innodb_buffer_pool_size' variable.
- Improved calculation of the 'key_buffer_size' variable.
- Improved calculation of the 'innodb_buffer_pool_instances' variable. Closes #37.

Releem MySQLConfigurer 0.4.0, 2020-11-21
- Improved documentation
- Added option -m to set memory limit for MySQL in MBs. Closes #42.
- Fixed downloading MySQLTuner every launch. Closes #46.
- Added option -k - Releem API Key authorization.
- Created Releem Community groups on Slack and Telegram.

MySQL Configurer 0.3.2, 2020-08-24
- Added MySQL 8 support. Closes #39 
- Fixed calculation of the 'key_buffer_size' variable for MySQL 8.0.
- Tested compatibility with MySQL 5.5, MySQL 5.6, MySQL 5.7, MySQL 8.0, MariaDB 10.1, MariaDB 10.2, MariaDB 10.3, MariaDB 10.4, MariaDB 10.5.
- Improved documentation with Security section.
- Improved documentation with information about setting open_files_limit.
- Improved documentation with installation perl-Data-Dumper module on Centos.

MySQL Configurer 0.3.1, 2020-07-08
- Added calculation of the 'table_open_cache' variable. 
- Added calculation of the 'table_definition_cache' variable. Closes #18

MySQL Configurer 0.3.0, 2020-06-24
- Tested compatibility with MySQL 5.5, MySQL 5.6, MySQL 5.7, MariaDB 10.1, MariaDB 10.2, MariaDB 10.3.
- Added calculation of the 'key_buffer_size' variable for improve performance of the MyIsam storage engine.
- Added calculation of the 'innodb_buffer_pool_chunk_size' variable for MySQL 5.7.5 and later, MariaDB 10.2.2 and later.
- Added calculation of the 'max_connections' variable based on 'Max_used_connections' MySQL status variable.
- Improve calculation of the 'innodb_log_file_size' variable using 'innodb_log_files_in_group' variable.
- Improve documentation with install dependencies step for Debian/Ubuntu and Centos/Redhat.
- Fix documentation. Update example of the recommended configuration file. Closes #35
- Fix documentation. How to safely apply the configuration file. Closes #36

MySQL Configurer 0.2.2, 2020-04-25
- Improve documentation. Added supported MySQL versions. Closes #22
- Imrove stability. Response message for incompatible report. Closes #10

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