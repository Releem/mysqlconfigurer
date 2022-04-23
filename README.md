# Releem MySQLConfigurer

[![Build Status - Master](https://travis-ci.com/releem/mysqlconfigurer.svg?branch=master)](https://travis-ci.com/releem/mysqlconfigurer)


## Description
**MySQLConfigurer** is a script that will assist you prepare performance optimized configuration of your MySQL server based on the MySQLTuner report (MySQL status and system information). 

**Releem** is an online service for automatic optimization MySQL configuration to improve performance and reduce costs. Releem analyzes the MySQLTuner report, MySQL status and system information of your server and provides settings recommendations in the form of a MySQL configuration file.

To get your Releem API Key please [sign up](https://releem.com/?utm_source=github&utm_medium=link&utm_campaign=signup#rec221377760).

## Support
Join the Releem Community on [Slack](https://mysqlcommunity.slack.com/archives/C01FFDYTWTW) and [Telegram](https://t.me/releemhq). 

## Features
- Fully automated MySQL performance configuration tuning. 
- Supports all MySQL/MariaDB versions.
- **MySQLConfigurer** recommended configuration deliver a [30% boost](#Tests) to MySQL performance compare to the default configuration.
- **MySQLConfigurer** supports 25 parameters of MySQL/Percona/MariaDB server.
- Using **MySQLConfigurer** you can tune configuration of MySQL server just in [60 seconds](https://youtu.be/7gixIYTpuPU).
- You could use **MySQLConfigurer** to get the recommended values for your server and insert in your configuration.

## Warning
**Always** test recommended configuration on staging environments, and **always** keep in mind that improvements in one area can **negatively** affect MySQL in other areas.

It's also important to wait at least a day of uptime to get accurate results.

## Security
**Always** store credentials only in `~/.my.cnf` to prevent sending passwords to our service.
In other cases MySQLTuner stores passwords manualy entered while running script in "MySQL Client" section of the MySQLTuner report.

## Compatibility
- MySQL 8.0
- MySQL 5.7
- MySQL 5.6
- MySQL 5.5
- MariaDB 10.1
- MariaDB 10.2
- MariaDB 10.3
- MariaDB 10.4
- MariaDB 10.5
- MariaDB 10.6

*** MINIMAL REQUIREMENTS ***
- Perl 5.6 or later (with perl-doc package)
- Perl module JSON
- Unix/Linux based operating system (tested on Linux, BSD variants, and Solaris variants)
- Unrestricted read access to the MySQL server (OS root access recommended for MySQL < 5.1)

## Technical details
**MySQLConfigurer** is a Bash script which
1. downloads last version of the MySQLTuner
2. runs MySQLTuner with options "--json --verbose --notbstat"
3. uploads MySQLTuner report in the JSON to Releem online service https://api.servers-support.com/v1/mysql
4. downloads recommended MySQL config file

## Tests
We tested the results with Sysbench on a virtual server running Debian 9 (2 CPU, 2GB Ram) the table contained 10 million entries.
Two configurations were tested, the MySQL default configuration and the configuration recommended by the **Releem** service. The tests were two-step: read (test1) only and read/write (test2).

Recommended configuration delivered a 30% boost to MySQL performance compared to the default configuration. 

Follow this links to see results:
[MySQL 5.7 Benchmark](https://releem.com/blog/how-to-improve-performance-mysql57-default-configuration)
[MySQL 8 Benchmark](https://releem.com/blog/tpost/9kdjxj8ve1-mysql-8-performance-benchmark)

## Options
**-k [Releem API KEY]** - API Key to Releem platform. To get your Releem API Key please [sign up](https://releem.com/?utm_source=github&utm_medium=link&utm_campaign=signup#rec221377760).

**-m [MYSQL_MEMORY_LIMIT]** - set maximum memory limit for MySQL. Used when there are installed different applications on the server.

## Installation

1. One step installation to /opt/releem
    ```
    RELEEM_API_KEY=[YOUR_RELEEM_API_KEY] bash -c "$(curl -L https://releem.s3.amazonaws.com/install.sh)"
    ```
2. Create ~/.my.cnf file with the following content:
    ```
    [client]
    user=root
    password=[your password]
    ```
3. Run Releem Agent manually to send first metrics from your server:
    ```bash
    /bin/bash /opt/releem/mysqlconfigurer.sh -k [RELEEM_API_KEY]
    ```
    - Use -m [MYSQL_MEMORY_LIMIT] - to set maximum memory limit for MySQL. Used in case you are using MySQL in Docker or it isn't dedicated server for MySQL.
    - **RELEEM_API_KEY** - To get your Releem API Key please [sign up](https://releem.com/?utm_source=github&utm_medium=link&utm_campaign=signup#rec221377760).
4. Add command from step 3 in the crontab to get better recommendations. For example:
    ```
    10 */12 * * * /bin/bash /opt/releem/mysqlconfigurer.sh -k [RELEEM_API_KEY]
    ```

## When should I apply the Recommended Configuration?

Releem recommends the best performance optimized MySQL configuration at this time.
You can apply Recommended Configuration in case of MySQL Performance Score of your server goes down. 

## How to apply the Recommended configuration

Releem Agent automatically stores Recommended Configuration at /tmp/.mysqlconfigurer/z_aiops_mysql.cnf

Perform the following steps to safely apply recommended configuration:

1. Copy the Recommended Configuration to MySQL configuration folder:
    ```
        cp /tmp/.mysqlconfigurer/z_aiops_mysql.cnf  /etc/mysql/conf.d/
    ```
    * The path to `/etc/mysql/conf.d` folder can vary according to Linux distro.
    * In CentOS instead /etc/mysql/conf.d you should use /etc/my.cnf.d/ folder.

2. Restart MySQL to apply the Recommended configuration:
    ```
        service mysqld restart
    ```
    **WARNING!** **In case of change 'innodb_log_file_size' only in MySQL 5.6.7 or earlier** set parameter 'innodb_fast_shutdown' to 1 ([Official documentation](https://dev.mysql.com/doc/refman/5.6/en/innodb-redo-log.html)), stop MySQL server, copy old log files into a safe place and delete it from log directory, copy recommended configuration and start MySQL server: 
    ```bash
        mysql -e"SET GLOBAL innodb_fast_shutdown = 1"
        service mysql stop
        mv /var/lib/mysql/ib_logfile[01] /tmp
        service mysql start
    ```

## How to increase `open_files_limit`

Perform the folowing steps to safely setup `open_files_limit` in MySQL

1. Find out if any other .conf files are being used with MySQL that overrides the values for open limits. Run `systemctl status mysqld/mysql/mariadb` command and it will show something like this
    ```
        Drop-In:
            /etc/systemd/system/(mysqld/mysql/mariadb).service.d
            └─limits.conf
    ```
        
    This means there is `/etc/systemd/system/(mysqld/mysql/mariadb).service.d/limits.conf` file which is loaded with MySQL Server. If this file does not exist, you should create create it.
    
    `mysqld/mysql/mariadb` is selected depending on the name of the running service name on the server, which is also defined in the output of the command `systemctl status mysqld/mysql/mariadb`

2. Edit the file and add the following and change `[table_open_cache]` to your value
    ```
        [Service]
        LimitNOFILE=([table_open_cache] * 2)
    ```
    - **`open_files_limit` should be no less than `[table_open_cache] * 2`.**

3. Run the following command to apply the changes.
        `systemctl daemon-reload`

4. Reboot your mysql server.
    
5. After the successful reboot of the server, we will again run below SQL Queries.

    ```
        SHOW VARIABLES LIKE 'open_files_limit';
    ```
        
    You should see the following:
        
    ```
        +------------------+--------+
        | Variable_name    | Value  |
        +------------------+--------+
        | open_files_limit | 102400 |
        +------------------+--------+
        1 row in set (0.00 sec)
    ```


## Example of Recommended MySQL Configuration

Example of the recommended configuration file /tmp/.mysqlconfigurer/z_aiops_mysql.cnf:
```
[mysqld]
query_cache_type = 1 ### Previous value : ON
query_cache_size = 128M ### Previous value : 134217728
query_cache_limit = 16M ### Previous value : 16777216
thread_cache_size = 8 ### Previous value : 8
key_buffer_size = 205520896 ### Previous value : 205520896
max_allowed_packet = 1073741824 ### Previous value : 67108864
sort_buffer_size = 16777216 ### Previous value : 25165824
read_rnd_buffer_size = 4194304 ### Previous value : 4194304
bulk_insert_buffer_size = 8M ### Previous value : 2097152
myisam_sort_buffer_size = 8388608 ### Previous value : 25165824
innodb_buffer_pool_instances = 2 ### Previous value : 3
innodb_buffer_pool_size = 3019898880 ### Previous value : 3019898880
max_heap_table_size = 256M ### Previous value : 268435456
tmp_table_size = 256M ### Previous value : 268435456
join_buffer_size = 8M ### Previous value : 8388608
max_connections = 151 ### Previous value : 151
table_open_cache = 3072 ### Previous value : 3072
table_definition_cache = 1920 ### Previous value : 1920
innodb_flush_log_at_trx_commit = 2 ### Previous value : 2
innodb_log_file_size = 377487360 ### Previous value : 805306368
innodb_write_io_threads = 4 ### Previous value : 4
innodb_read_io_threads = 4 ### Previous value : 4
innodb_file_per_table = 1 ### Previous value : ON
innodb_flush_method = O_DIRECT ### Previous value :
innodb_thread_concurrency = 0 ### Previous value : 0
```

## Contribute

You can help this project by reporting problems, suggestions or contributing to the code.

### Report a problem or suggestion

Go to our [issue tracker](https://github.com/releem/mysqlconfigurer/issues) and check if your problem/suggestion is already reported. If not, create a new issue with a descriptive title and detail your suggestion or steps to reproduce the problem.
