#!/bin/bash

wget http://mysqltuner.pl/ -O mysqltuner.pl

perl mysqltuner.pl --json --verbose --notbstat --outputfile='mysqltunerreport.json'

curl -d "@/root/mysqltuner/1.json" -X POST https://z3sog3l1l7.execute-api.us-east-1.amazonaws.com/dev/mysqltunerreport