# Releem Agent

[![Go Report Card](https://goreportcard.com/badge/github.com/Releem/mysqlconfigurer)](https://goreportcard.com/report/github.com/Releem/mysqlconfigurer)
[![slack](https://img.shields.io/badge/slack-Releem%20Community-brightgreen.svg?logo=slack)](https://join.slack.com/t/releem-community/shared_invite/zt-1j3d0vosh-AJHbDiQrzVDvLat5eqQorQ)

<p align="center">
  <a href="https://releem.com/">
    <img src="https://raw.githubusercontent.com/releem/docs/master/assets/images/releem-icon-top.png" width="140px" alt="Releem" />
  </a>
</p>
<p align="center">
    <a href="https://releem.com/docs/getstarted">Docs<a> | 
    <a href="https://releem.com/compare/mysqltuner">Compare MySQLTuner</a> |
    <a href="https://releem.com/sql-query-optimization">SQL Query Optimization</a> |
    <a href="https://releem.com/blog">Blog</a>
    <br /><br />
</p>



The present repository contains the source code of the **Releem Agent**.

[Releem](https://releem.com) is a MySQL Performance Management tool that makes managing and tuning database servers easy. Releem will assist you in MySQL configuration tuning, SQL query optimization and index management, query analytics and root cause analysis.

With Releem we are trying to bring top-notch experience in database performance management and save thousands of software engineers hours.

<p align="center">
<img src="https://raw.githubusercontent.com/releem/docs/master/assets/images/releem_dashboard.png" width="90%">
</p>

## Why Releem?
- **Clutter Free**: Releem provides simple dashboard and it cuts through the noise. No layers of menus, no need for custom reports. Get all the important metrics on one single page. No training necessary.
- **Hassle free**: Simple one-step Installation on most popular Linux platforms and Support of all MySQL/MariaDB/Percona versions.
- **Performance Booster**: Recommended configuration delivers up to [290% boost](#Tests) to MySQL performance compare to the default configuration.
- **Simplified Monitoring**: [MySQL Health Checks](https://releem.com/blog/mysql-health-checks?utm_source=github&utm_medium=social&utm_campaign=mysql-health-checks&utm_content=post) greatly simplifies the process of monitoring and maintaining a healthy database by focusing on key aspects that describe the efficiency and "best practices" of using Memory, Connections, Logs, Cache, Disk, Indexes, and Threads. Releem Score metric calculates by summarizing Health Checks statuses.
- **Automatic SQL Query Optimization and Index suggestions**: Releem automatically identifies inefficient queries and offers missed indexes, enabling database administrators to boost query performance without extensive manual analysis.
- **Security**: Security is our top priority. Releem does not use your database data. It uses only MySQL metrics and system information, and HTTPS to transfer them.
Releem Agent is open-source and can be reviewed to ensure it meets your security requirements.
- **Email report**: Keep an eye on your servers with weekly email reports.
- **Simple Applying**: Releem Agent allows simply apply recommended MySQL configuration just in one click or in one command.
<p align="center">
<img src="https://raw.githubusercontent.com/releem/docs/master/assets/images/releem-applying-click.gif" width="80%">
</p>

<p align="center">
<img src="https://raw.githubusercontent.com/releem/docs/master/assets/images/releem-applying.gif" width="80%">
</p>

## How it works

**Releem Agent** - Has been installed on servers, collects MySQL metrics, sends them to Cloud Platforms, and applies MySQL configurations. Open Source daemon built on Go.

**Releem Cloud Platform** - Analyzes collected metrics, detects performance issues, and recommends MySQL configurations.

**Releem Customer Portal** - Web interface displays recommended configurations and current information about all MySQL servers with installed Releem Agent. It looks like this on the screenshot.

## Getting started with Releem
The easiest way to get started with Releem is with [our  managed service in the cloud](https://releem.com) and one step installation command. It takes up to 5 minutes to start monitoring your MySQL servers and get recommendations to improve performance.

To start using Releem just sign up at [https://releem.com](https://releem.com/?utm_source=github&utm_medium=link&utm_campaign=signup#) and install Releem Agent on your server.

## Support
Join the Releem Community on [Slack](https://join.slack.com/t/releem-community/shared_invite/zt-1j3d0vosh-AJHbDiQrzVDvLat5eqQorQ). 

## Compatibility
- MySQL 8.0, MySQL 5.7, MySQL 5.6, MySQL 5.5
- MariaDB 10.1, MariaDB 10.2, MariaDB 10.3, MariaDB 10.4, MariaDB 10.5, MariaDB 10.6, MariaDB 10.7, MariaDB 10.8, MariaDB 10.9, MariaDB 10.10, MariaDB 10.11, MariaDB 11.0
- Percona 8.0, Percona 5.7, Percona 5.6, Percona 5.5
- Centos, CloudLinux, Debian, Ubuntu, RockyLinux
- Amazon RDS MySQL, Amazon RDS Aurora, Amazon RDS MariaDB

*** MINIMAL REQUIREMENTS ***
- Unix/Linux based operating system (tested on Linux, BSD variants, and Solaris variants)
- Unrestricted read access to the MySQL server

## Tests
We tested the results with Sysbench on a virtual server running Debian 9 (2 CPU, 2GB Ram) the table contained 10 million entries.
Two configurations were tested, the MySQL default configuration and the configuration recommended by the **Releem** service. The tests were two-step: read (test1) only and read/write (test2).

Recommended configuration delivered a 30% boost to MySQL performance compared to the default configuration. 

Follow this links to see results:
- [MySQL 5.7 Benchmark](https://releem.com/blog/how-to-improve-performance-mysql57-default-configuration)
- [MySQL 8 Benchmark](https://releem.com/blog/mysql-8-performance-benchmark)
- [How MySQL Configuration Impacts the Performance of Web Applications](https://releem.com/blog/web-applications-performance)

## Feedback 
We welcome feedback from our community. Take a look at our [feedback board](https://releem.com/wall-of-love). Please let us know if you have any requests and vote on open issues so we can better prioritize.

To stay up to date with all the latest news and product updates, make sure to follow us on [Twitter](https://twitter.com/releemhq), [LinkedIn](https://www.linkedin.com/company/releem).

## Contribute

You can help us by reporting problems, suggestions or contributing to the code.

### Report a problem or suggestion

Go to our [issue tracker](https://github.com/releem/mysqlconfigurer/issues) and check if your problem/suggestion is already reported. If not, create a new issue with a descriptive title and detail your suggestion or steps to reproduce the problem.
