if [ -z "$1" ]; then
    echo "Usage: $0 <docker repository name>"
    echo "Please provide the repository name as the first argument."
    exit 1
fi

REPO="$1"

#Delete previous images
docker image rm tungo:latest "$REPO"/tungo:tungo -f

#Build new image
docker buildx build -t tungo ../src/

#Tag new image
docker tag tungo:latest "$REPO"/tungo:tungo

#Push new image
docker push "$REPO"/tungo:tungo