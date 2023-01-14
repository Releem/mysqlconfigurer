FROM debian

ARG DB_HOST
ARG DB_PORT
ARG DB_PASSWORD
ARG DB_USER

ARG RELEEM_API_KEY
ARG MEMORY_LIMIT

ARG INSTANCE_TYPE
ARG AWS_REGION
ARG AWS_RDS_DB

ARG RELEEM_ENV
ARG RELEEM_DEBUG

RUN apt update \
 && apt install -y \
 curl \
 mariadb-client \
 net-tools \
 libjson-perl \
 procps \
 iputils-ping

RUN curl -L https://github.com/a8m/envsubst/releases/download/v1.2.0/envsubst-`uname -s`-`uname -m` -o envsubst \
 && chmod +x envsubst \
 && mv envsubst /usr/local/bin

WORKDIR /opt/releem
RUN mkdir /opt/releem/conf

COPY docker/ /docker/

RUN curl -o releem-agent https://releem.s3.amazonaws.com/v2/releem-agent \
 && curl -o mysqlconfigurer.sh https://releem.s3.amazonaws.com/v2/mysqlconfigurer.sh \
 && chmod +x releem-agent mysqlconfigurer.sh

RUN mkdir -p /etc/mysql/releem.conf.d

ENTRYPOINT [ "/docker/entrypoint.sh" ]
CMD ["/opt/releem/releem-agent"]
