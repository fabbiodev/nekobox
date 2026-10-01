#!/bin/bash
. script/env_deploy.sh

set -e
export NEKOBOX_ENV_DEPLOYED=yes

if [[ -z `command -v $GOCMD` ]]
then
 export SKIP_BUILD_GO=yes
fi

if [[ -d "$1" ]]
then
 export BUILD="$1"
 export DEST="$1"
fi

if [[ -d download-artifact && "$SKIP_BUILD_GO" != yes ]]
then
 for source_artifact in download-artifact/*-Common-public-public; do
   if [[ -f "$source_artifact/artifacts.tgz" ]]; then
     (
       cd "$source_artifact"
       tar xvzf artifacts.tgz -C .
       mkdir -p "$DEPLOYMENT"
       mv deployment/* "$DEPLOYMENT"
     )
   fi
 done
fi

# Binary-only releases build directly from the checked-out Go module. Mount
# the deploy key at runtime; never copy it into the public source artifact.
if [[ -s /run/secrets/nekobox_core_key ]]; then
  install -d -m 700 /root/.ssh
  ssh-keyscan -t ed25519 github.com >> /root/.ssh/known_hosts
  export GIT_SSH_COMMAND="ssh -i /run/secrets/nekobox_core_key -o IdentitiesOnly=yes -o UserKnownHostsFile=/root/.ssh/known_hosts -o StrictHostKeyChecking=yes"
  git config --global url."git@github.com:1maxwarner/nekobox-sing-box-core".insteadOf "https://github.com/1maxwarner/nekobox-sing-box-core"
  export GOPRIVATE=github.com/1maxwarner/nekobox-sing-box-core
  export GONOSUMDB=github.com/1maxwarner/nekobox-sing-box-core
fi

echo $archive_standalone

if [[ -f $DEPLOYMENT/$archive_standalone.tar.xz && "$SKIP_BUILD_GO" != yes ]]
then
   pushd $DEPLOYMENT
   tar -xvf $archive_standalone.tar.xz
   ls $PWD
   echo $archive_standalone
   export SRC_ROOT=$PWD/$archive_standalone
   ln *.AppImage "$SRC_ROOT" ||:
#   BUILD="$SRC_ROOT/build"
   if [[ -d "$SRC_ROOT/core/server/vendor" ]]; then
     export GOFLAGS="-mod=vendor $GOFLAGS"
   fi
   export VERSION_SINGBOX="$(cat $SRC_ROOT/SingBox.Version)"
   export LAST_ACTION='rm -rf "$SRC_ROOT"'
   popd
else
  LAST_ACTION="echo fine"
fi 

if [[ "$SKIP_BUILD_GO" != yes ]]
then
  export BUILD_GO_PARTS=ON
else
  export BUILD_GO_PARTS=OFF
fi

(
echo "$SRC_ROOT"
cd "$SRC_ROOT"

cmake -S "$SRC_ROOT" -B "$BUILD" -GNinja -DNKR_DEFAULT_VERSION="${INPUT_VERSION:-5.0.0}" -DSKIP_UPDATER="${SKIP_UPDATE_BUTTON:-OFF}" -DBUILD_GO_PARTS="${BUILD_GO_PARTS}" -DGOOS="${GOOS}" -DGOARCH="${GOARCH}"
cmake --build "$BUILD" -v -j $(nproc)
(
. script/deploy_linux64.sh; 
)
)

eval "$LAST_ACTION"
