FROM debian

RUN apt update \
 && apt install -y \
 curl \
 mariadb-client \
 postgresql-client

# RUN curl -L https://github.com/a8m/envsubst/releases/download/v1.4.3/envsubst-`uname -s`-`uname -m | sed  's/aarch64/arm64/g'` -o envsubst \
#  && chmod +x envsubst \
#  && mv envsubst /usr/local/bin

WORKDIR /opt/releem
RUN mkdir /opt/releem/conf

COPY docker/ /docker/

RUN curl -L -o releem-agent https://releem.s3.amazonaws.com/v2/releem-agent-$(arch) \
 && curl -L -o mysqlconfigurer.sh https://releem.s3.amazonaws.com/v2/mysqlconfigurer.sh \
 && chmod +x releem-agent mysqlconfigurer.sh /docker/entrypoint.sh
 
RUN mkdir -p /etc/mysql/releem.conf.d

ENTRYPOINT [ "/docker/entrypoint.sh" ]
CMD ["/opt/releem/releem-agent"]
