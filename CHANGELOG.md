Releem releases
---
Information about releases of the Releem Agent.

Releem 0.9.7, 2022-10-31
- Added installation logs collection.
- Improved metrics collection using new Releem Agent implemented using Go.
- Improved installation. Removed cron to collect metrics.
- Redesigned front page of Releem Customer Portal.
- Fixed run installation with sudo user.

Releem 0.9.6, 2022-09-30
- Added Queries per Second metric collection
- Added graph QPS in Releem Customer Portal
- Redesigned Dashboard in Releem Customer Portal
- Added Weekly Reelem Reports
- Improved server metrics aggregation algorithm (hostnames instead IP adresses) Closes #100
- Fixed warning during execution. Closes #93

Releem 0.9.5, 2022-08-31
- Added Apply recommended MySQL configuration and rollback to previous configuration. Closes #63
- Added Automatic update.
- Added innodb_page_cleaners and innodb_purge_threads calculations.
- Improved performance of Releem Agent minimize workload an run on servers with hundreds databases. Closes #30

Releem 0.9.4, 2022-07-31
- Added FreeBSD support. Closes #95
- Added innodb_redo_log_capacity calculation
- Added query_cache_min_res_unit calculation
- Improved calculation thread_cache_size. Closes #91
- Fixed Error is:'int' object has no attribute 'strip'
- Fixed Error KeyError: 'Virtual Machine'

Releem 0.9.3, 2022-06-30
- Added thread_pool_size calculation.
- Improved performance of metrics page.
- Improved [MySQL Performance Score](https://releem.com/docs/mysql-performance-score?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md).
- Fixed [innodb_buffer_pool_size](https://releem.com/docs/mysql-performance-tuning/innodb_buffer_pool_size?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md) calculation.
- Fixed height of Recommended Configuration block.

Releem 0.9.2, 2022-05-31
- Added manual selection of [innodb_flush_log_at_trx_commit](https://releem.com/docs/mysql-performance-tuning/innodb_flush_log_at_trx_commit?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md) in Releem Customer Portal for every server.
- Added innodb_log_buffer_size calculaction.
- Added optimizer_search_depth calculaction.
- Improved [innodb_log_file_size](https://releem.com/docs/mysql-performance-tuning/innodb_log_file_size?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md) variable. Closes #3
- Improved [Documentation](https://releem.com/docs/getstarted).
- Fixed Metrics Collection Issue.

Releem Agent 0.9.1, 2022-04-30
- Added display of Memory Limit in Releem Customer Portal
- Improved [MySQL Performance Score](https://releem.com/docs/mysql-performance-score?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md)
- Fixed duplicated servers in Releem Customer Portal
- Removed servers where Releem Agent is not active.

Releem MySQLConfigurer 0.9.0, 2022-03-30
- Added checks of the database server version
- Added configuration file releem.conf
- Added -u option to update Releem Agent
- Added list of variable changes. Closes #75
- Improved calculation of [max_heap_table_size](https://releem.com/docs/mysql-performance-tuning/max_heap_table_size?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md)
- Improved calculation of [tmp_table_size](https://releem.com/docs/mysql-performance-tuning/tmp_table_size?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md) 
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
- Added [MySQL Performance Score](https://releem.com/docs/mysql-performance-score?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md) metric.
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
- Added calculation of the '[max_allowed_packet](https://releem.com/docs/mysql-performance-tuning/max_allowed_packet?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md)' variable.
- Added calculation of the 'read_rnd_buffer_size' variable.
- Improved calcualtion of the 'sort_buffer_size' variable.
- Improved calculation of the '[innodb_buffer_pool_size](https://releem.com/docs/mysql-performance-tuning/innodb_buffer_pool_size?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md)' variable.
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
- Added calculation of the '[table_open_cache](https://releem.com/docs/mysql-performance-tuning/table_open_cache)' variable. 
- Added calculation of the 'table_definition_cache' variable. Closes #18

MySQL Configurer 0.3.0, 2020-06-24
- Tested compatibility with MySQL 5.5, MySQL 5.6, MySQL 5.7, MariaDB 10.1, MariaDB 10.2, MariaDB 10.3.
- Added calculation of the '[key_buffer_size](https://releem.com/docs/mysql-performance-tuning/key_buffer_size)' variable for improve performance of the MyIsam storage engine.
- Added calculation of the '[innodb_buffer_pool_chunk_size](https://releem.com/docs/mysql-performance-tuning/innodb_buffer_pool_chunk_size)' variable for MySQL 5.7.5 and later, MariaDB 10.2.2 and later.
- Added calculation of the '[max_connections](https://releem.com/docs/mysql-performance-tuning/max_connections)' variable based on 'Max_used_connections' MySQL status variable.
- Improve calculation of the '[innodb_log_file_size](https://releem.com/docs/mysql-performance-tuning/innodb_log_file_size)' variable using 'innodb_log_files_in_group' variable.
- Improve documentation with install dependencies step for Debian/Ubuntu and Centos/Redhat.
- Fix documentation. Update example of the recommended configuration file. Closes #35
- Fix documentation. How to safely apply the configuration file. Closes #36

MySQL Configurer 0.2.2, 2020-04-25
- Improve documentation. Added supported MySQL versions. Closes #22
- Imrove stability. Response message for incompatible report. Closes #10

MySQL Configurer 0.2.1, 2020-04-11
- Fixed rename file z_aiops_mysql.conf -> z_aiops_mysql.cnf. Issue #14 was closed
- Added rounding of variables. Issue #17 was closed.
- Added calculation '[max_connections](https://releem.com/docs/mysql-performance-tuning/max_connections?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md)'. Issue #16 was closed.
- Added calculation '[thread_cache_size](https://releem.com/docs/mysql-performance-tuning/thread_cache_size?utm_source=github&utm_medium=social&utm_campaign=changelog&utm_content=md)'. Issue #15 was closed.
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