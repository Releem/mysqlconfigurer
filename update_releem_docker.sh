#!/bin/bash
NAME_CONTAINER=$1

printf "`date +%Y%m%d-%H:%M:%S`\033[32m Agent Update Start! \033[0m\n"


VERSION=$(docker inspect  -f '{{range $i, $v := split .Config.Image ":"}}{{if eq  $i  1}}{{println  $v}}{{end}}{{end}}' ${NAME_CONTAINER})
NEW_VER=$(curl  -s -L https://releem.s3.amazonaws.com/v2/current_version_agent)
if [ "$VERSION" \< "$NEW_VER" ]
then
    printf "`date +%Y%m%d-%H:%M:%S`\033[32m Updating script \e[31;1m%s\e[0m -> \e[32;1m%s\e[0m\n" "$VERSION" "$NEW_VER"
    enviroment_docker=$(docker inspect  -f '{{ join .Config.Env " -e " }}' ${NAME_CONTAINER})
    docker_run="docker run -d -ti --name  ${NAME_CONTAINER} -e ${enviroment_docker} releem/releem-agent:${NEW_VER}"
    echo "$docker_run"
    docker rm -f $NAME_CONTAINER
    eval "$docker_run"
    printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m Releem Agent updated successfully.\033[0m\n"
else
    printf "\n`date +%Y%m%d-%H:%M:%S`\033[32m Agent update is not required.\033[0m\n"
fi


