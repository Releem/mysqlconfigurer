# Release Notes for tags `1.2*.*`

## 1.20.0
- Initial tag in selected range
- Optimized database list collection

## 1.21.0
- Increased version
- Added collect processlist
- Enabled logging for Windows
- Added Security section to Readme
- New version release 1.19.9

## 1.21.1
- Users security checks (#405)

## 1.21.2
- Updated algorithm for blank password detection

## 1.21.3
- Fixed error when plugin `validate_password` is activated
- Added 1.20 release documentation updates

## 1.21.3.1
- Added the ability to select the region for storing server data
- Increased agent version

## 1.21.3.2
- Hotfix: converted variable types
- Fixed error converting `driver.Value` `[]uint8` to int (value out of range)

## 1.21.3.3
- Increased version
- Fixed algorithm for generating `Password` column for security checks
- Fixed calculation of query example count

## 1.21.3.4
- Fixed bug when there are no privileges for the table

## 1.21.4
- V1.21.4 (#416)

## 1.21.4.1
- Corrected installation process text

## 1.21.4.2
- Updated project Go version

## 1.21.5
- Improved security: user information is not sent to platform
- User checks are implemented on the agent side

## 1.21.6
- Optimized latency collection (#434)
- Releem 1.21.0 changelog updates

## 1.21.6.1
- Added query length limitation to processlist

## 1.22.0
- Increased version
- Optimized query examples collection query
- Fixed `need_grant_permission` check
- Added release notes to changelog
- Added all Releem Agent parameters

## 1.22.0.1
- Added SSL support for Docker and RDS
- Added tracking updates

## 1.22.0.2
- Increased version
- Improved agent installation (#455), including manual RDS-on-EC2 option
- Optimized process list collection (#456)

## 1.22.0.3
- Increased version
- Added GCP Cloud SQL feature (#458)

## 1.22.0.4
- Increased version
- Added initial config apply flow (discussion #465)

## 1.22.0.5
- Increased version
- Fixed TLS usage
- Added release 1.22.0 to changelog

## 1.22.1
- Fixed collection for all text queries
- Updated `CHANGELOG.md` for 1.22.0.4
- Added blank line before stages declaration

## 1.22.2
- Increased version
- Added `information_schema.key_column_usage` collection for foreign key checks
- Added `performance_schema_digests_size` variable setup in installer

## 1.22.2.1
- Fixed initial configuration apply flow

## 1.22.2.2
- Fixed Windows crash: `invalid memory address or nil pointer dereference`

## 1.22.3
- Added collection of additional tables for automatic schema-change apply
- Fixed error handler

## 1.22.3.1
- Fixed permission-check condition

## 1.22.3.2
- Enabled query interpolation
- Added release 1.22.1 to changelog

## 1.22.4
- Fixed `releem.conf` template
- Increased version
- Updated Go and dependency versions
- Removed `envsubst` dependency

## 1.22.5
- Changed query filter field to avoid full table scan

## 1.22.6
- Optimized `events_statements_*` consumer enablement
- Optimized statement samples collection

## 1.22.7
- Fixed table count collection
- Optimized `performance_schema.table_io_waits_summary_by_index_usage` collection

## 1.23.0
- Refactored code
- Added PostgreSQL support
- Optimized custom query flow (#498)

## 1.23.1
- Fixed debug message
- Fixed enabling query optimization

## 1.23.2
- Increased version
- Optimized index usage statistics collection
- Updated error texts

## 1.23.3
- Increased version
- Fixed filtering field
- Added `Last_Seen` check for EXPLAIN collection
- Added skipping empty partition (issue #495)

## 1.23.3.1
- Increased version
- Refactored bash scripts and added bash unit tests (#505)
- Updated changelog for 1.22.7 and related fixes/features (#501)

## 1.23.4
- Updated version
- Updated package versions
- Updated Go and dependencies
- PostgreSQL query optimization (#507)
- Fixed error when `metrics.DB.Conf.Variables` is nil
- Fixed timer for starting data collection

## 1.23.4.1
- Added serverless database capacity metric
- Bumped agent version to 1.23.4.1
- Added support for Aurora Serverless v2 Enhanced Monitoring metrics (#508)

## 1.23.5
- Increased versions
- Bumped Go toolchain and module dependencies
- Enabled `mysql_ssl_mode=true` by default for Azure MySQL installs
- Fixed debug message
- Switched config requests to queries API domain
- Split apply flow by platform and added Azure MySQL support
- Added fail-fast behavior on startup errors
- Included related changelog and test updates
