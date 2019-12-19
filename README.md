# MySQL configurer

**Attention. Service is in alfa version and mysql.conf file you should use for your own risk.**

## Description
MySQL AIOps tool for automatic config generation based on MySQLtuner recommendations

## Technical details
This is simple Bash script which
1. download last version of the MySQLTuner
2. run MySQLTuner with options "--json --verbose --notbstat"
3. upload MySQLTuner report in the JSON to AIOps online service https://api.servers-support.com/v1/mysql
4. download recommended MySQL config file

## Usage
1. Download mysqlconfigurer.sh
```bash
wget https://github.com/initlabopen/mysqlconfigurer/blob/master/mysqlconfigurer.sh
```
2. Run mysqlconfigurer.sh
```bash
/bin/bash mysqlconfigurer.sh
```
3. In the /tmp/.mysqlconfigurer folder you could see
```bash
root@mysqlconfigurer# ls -l /tmp/.mysqlconfigurer/
total 264
-rw-r--r-- 1 root root    479 Dec 19 06:03 z_aiops_mysql.conf
-rw-r--r-- 1 root root 226002 Dec 18 16:44 mysqltuner.pl
-rw-r--r-- 1 root root  33410 Dec 18 16:44 mysqltunerreport.json
```
- **mysqltunerreport.json** - the MySQLTuner report file in the JSON format
- **z_aiops_mysql.conf** - recommended MySQL config file downloaded from api.server-support.com

4. If you want to use this mysql.conf file you could copy it in the /etc/mysql/conf.d/ directory and restart MySQL server
```bash
cp /tmp/.mysqlconfigurer/z_aiops_mysql.conf  /etc/mysql/conf.d/
service mysql restart
```

## Contribute

You can help this project by reporting problems, suggestions or contributing to the code.

### Report a problem or suggestion

Go to our [issue tracker](https://github.com/initlabopen/mysqlconfigurer/issues) and check if your problem/suggestion is already reported. If not, create a new issue with a descriptive title and detail your suggestion or steps to reproduce the problem.
