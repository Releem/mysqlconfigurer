# MySQL configurer

[![Build Status - Master](https://travis-ci.com/initlabopen/mysqlconfigurer.svg?branch=master)](https://travis-ci.com/initlabopen/mysqlconfigurer)

## Description
**MySQL Configurer** is a script and online service to assist you prepare performance optimized configuration of your MySQL server based on the MySQLTuner report (MySQL status and system information). The service analyzes the MySQLTuner report, MySQL status and system information of your server and provides settings recommendations in the form of a MySQL configuration file.

## Features
- Fully automated MySQL performance optimized configuration creation. 
- **MySQL configurer** recommended configuration deliver a [30% boost](#Tests) to MySQL performance compare to the default configuration.
- **MySQL Configurer** supports 19 parameters of MySQL/Percona/MariaDB server.
- With **MySQL Configurer** you could prepare configuration file for your MySQL server just in [60 seconds](https://youtu.be/QluJpSl6dGk).
- You could use **MySQL Configurer** for getting the recommended values for your server and insert in your configuration.

## Warning
**Always** test recommended configuration on staging environments, and **always** keep in mind that improvements in one area can **negatively** affect MySQL in other areas.

It's also important to wait at least a day of uptime to get accurate results.

## Security
**Always** store credentials only in `~/.my.cnf` to prevent sending passwords to our service.
In other cases MySQLTuner stores passwords manualy entered while running script in "MySQL Client" section of the MySQLTuner report.

To use .my.cnf file create file `~/.my.cnf` with folowing content:
```
[client]
user=root
password=[your password]
```


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

*** MINIMAL REQUIREMENTS ***
- Perl 5.6 or later (with perl-doc package)
- Perl module JSON
- Unix/Linux based operating system (tested on Linux, BSD variants, and Solaris variants)
- Unrestricted read access to the MySQL server (OS root access recommended for MySQL < 5.1)

## Technical details
**MySQL Configurer** is a Bash script which
1. downloads last version of the MySQLTuner
2. runs MySQLTuner with options "--json --verbose --notbstat"
3. uploads MySQLTuner report in the JSON to AIOps online service https://api.servers-support.com/v1/mysql
4. downloads recommended MySQL config file

## Tests
We tested the results with Sysbench on a virtual server running Debian 9 (2 CPU, 2GB Ram) the table contained 10 million entries.
Two configurations were tested, the MySQL default configuration and the configuration recommended by the **MySQLConfigurer** service. The tests were two-step: read (test1) only and read/write (test2).

Recommended configuration delivered a 30% boost to MySQL performance compared to the default configuration. Follow this link to see test results:
https://docs.google.com/spreadsheets/d/1J9FDgBGbvNA356d74WKYBaEzSwK7H-wgjHEQgYh8CMI/edit?usp=sharing


## Usage
1. Install dependencies

	a. Debian/Ubuntu
	* `apt-get install -y wget curl net-tools libjson-perl`
	
	b. Centos/RedHat
	* `yum install -y wget curl net-tools perl-JSON perl-Data-Dumper`
	
	c. Other OS
	* `Requires installation of similar packages on your OS`
2. Download mysqlconfigurer.sh
    ```bash
    wget https://raw.githubusercontent.com/initlabopen/mysqlconfigurer/master/mysqlconfigurer.sh
    ```
    or
    ```bash
    curl -o mysqlconfigurer.sh  https://raw.githubusercontent.com/initlabopen/mysqlconfigurer/master/mysqlconfigurer.sh
    ```
3. Run mysqlconfigurer.sh
    ```bash
    /bin/bash mysqlconfigurer.sh
    ```
4. In the /tmp/.mysqlconfigurer folder you could see
    ```bash
    root@mysqlconfigurer# ls -l /tmp/.mysqlconfigurer/
    total 264
    -rw-r--r-- 1 root root    479 Dec 19 06:03 z_aiops_mysql.cnf
    -rw-r--r-- 1 root root 226002 Dec 18 16:44 mysqltuner.pl
    -rw-r--r-- 1 root root  33410 Dec 18 16:44 mysqltunerreport.json
    ```
    - **mysqltunerreport.json** - the MySQLTuner report file in the JSON format
    - **z_aiops_mysql.cnf** - recommended MySQL config file downloaded from api.server-support.com

5. **Only if you need to increase `open_files_limit` variable.** Perform the folowing steps to safely setup `open_files_limit` in MySQL

    5.1. Find out if any other .conf files are being used with MySQL that overrides the values for open limits. Run `systemctl status mysqld/mysql/mariadb` command and it will show something like this
    ```
        Drop-In:
            /etc/systemd/system/(mysqld/mysql/mariadb).service.d
            └─limits.conf
    ```
        
    This means there is `/etc/systemd/system/(mysqld/mysql/mariadb).service.d/limits.conf` file which is loaded with MySQL Server. If this file does not exist, you should create create it.
    
    `mysqld/mysql/mariadb` is selected depending on the name of the running service name on the server, which is also defined in the output of the command `systemctl status mysqld/mysql/mariadb`

    5.2. Edit the file and add the following and change `[table_open_cache]` to your value
    ```
        [Service]
        LimitNOFILE=([table_open_cache] * 2)
    ```
    - **`open_files_limit` should be no less than `[table_open_cache] * 2`.**

    5.3. Run the following command to apply the changes.
        `systemctl daemon-reload && /scripts/restartsrv_mysql`

    5.4. Reboot your mysql server.
    
    5.5. After the successful reboot of the server, we will again run below SQL Queries.

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

6. Perform the following steps to safely apply recommended configuration:
    
    **WARNING!** **In case of change 'innodb_log_file_size' only in MySQL 5.6.7 or earlier** set parameter 'innodb_fast_shutdown' to 1 ([Official documentation](https://dev.mysql.com/doc/refman/5.6/en/innodb-redo-log.html)), stop MySQL server, copy old log files into a safe place and delete it from log directory, copy recommended configuration and start MySQL server: 
    ```bash
        mysql -e"SET GLOBAL innodb_fast_shutdown = 1"
        service mysql stop
        cp /tmp/.mysqlconfigurer/z_aiops_mysql.cnf  /etc/mysql/conf.d/
        mv /var/lib/mysql/ib_logfile[01] /tmp
        service mysql start
    ```
    In other cases stop MySQL server, copy recommended configuration file and start MySQL server: 
    ```bash
        service mysql stop
        cp /tmp/.mysqlconfigurer/z_aiops_mysql.cnf  /etc/mysql/conf.d/
        service mysql start
    ```
    * The path to `/etc/mysql/conf.d` folder can vary according to Linux distro.

Example of the recommended configuration file /tmp/.mysqlconfigurer/z_aiops_mysql.cnf:
```
[mysqld]
query_cache_type = 1 ### Previous value : OFF
query_cache_size = 128M ### Previous value : 16777216
query_cache_limit = 16M ### Previous value : 1048576
thread_cache_size = 8 ### Previous value : 8
key_buffer_size = 196M ### Previous value : 268435456
sort_buffer_size = 24M ### Previous value : 1048576
bulk_insert_buffer_size = 2M ### Previous value : 8388608
myisam_sort_buffer_size = 24M ### Previous value : 67108864
innodb_buffer_pool_instances = 3 ### Previous value : 1
innodb_buffer_pool_size = 3019898880 ### Previous value : 1073741824
max_heap_table_size = 256M ### Previous value : 16777216
tmp_table_size = 256M ### Previous value : 16777216
join_buffer_size = 8M ### Previous value : 262144
max_connections = 151 ### Previous value : 151
interactive_timeout = 1200 ### Previous value : 28800
wait_timeout = 1200 ### Previous value : 28800
table_open_cache = 65536 ### Previous value : 4096
innodb_flush_log_at_trx_commit = 2 ### Previous value : 2
innodb_log_file_size = 805306368 ### Previous value : 67108864
```

## Contribute

You can help this project by reporting problems, suggestions or contributing to the code.

### Report a problem or suggestion

Go to our [issue tracker](https://github.com/initlabopen/mysqlconfigurer/issues) and check if your problem/suggestion is already reported. If not, create a new issue with a descriptive title and detail your suggestion or steps to reproduce the problem.
