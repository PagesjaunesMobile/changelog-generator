#!/bin/bash
set -e
set -x

THIS_SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cd $THIS_SCRIPT_DIR
npm install
gem install redcarpet
cd $BITRISE_SOURCE_DIR

if [ "$TAG_DEST" = "HEAD" ]; then
  tag_head=$(git tag --points-at HEAD)
  if [[ "$BITRISEIO_GIT_BRANCH_DEST" =~ (develop|master|release) ]]; then   
    previousTag=$(git rev-list --parents HEAD | head -1| cut -d' ' -f2)
  else
    previousTag=$(git merge-base HEAD origin/develop) 
  fi
  
  if [ -n "$tag_head" ]; then
    TAG_DEST=$tag_head
  fi
fi

if [ "$TAG_DEST" != "HEAD" ]; then
  git checkout ${TAG_DEST}~1
  previousTag=$(git describe --tags --abbrev=0)
fi

git checkout ${TAG_DEST}


if [ -n "$CHANGE_FILE" ] ; then
  if [ -z "$(git config user.name)" ]; then
    git config user.email $IC_COMMITER_MAIL
    git config user.name $IC_COMMITER_NAME
  fi
  
  touch $CHANGE_FILE
  ALREADY="$(grep "^# ${TAG_DEST}" $CHANGE_FILE || true)"
  
  # or ALREADY = tag <> HEAD
  if [ -z "$ALREADY" ]; then 
    $THIS_SCRIPT_DIR/changelog.js $TAG_DEST "${CHANGE_FILE}" $previousTag
    if $CI ; then
      GIT_ASKPASS=echo 
      GIT_SSH="${THIS_SCRIPT_DIR}/ssh_no_prompt.sh"
      git add $CHANGE_FILE
      git commit -m "chore(${TAG_DEST}):update changes"
      git push origin HEAD:$BITRISE_GIT_BRANCH
    fi
  fi
fi
#git log --invert-grep --grep="^Merge" -E --format=%H%n%s%n%b%n%an%n==END== ${previousTag}..${TAG_DEST}
changelog=$($THIS_SCRIPT_DIR/changelog.js $TAG_DEST '' $previousTag --lite)
changelog_final=$(echo "$($THIS_SCRIPT_DIR/changelog.js $TAG_DEST '' $previousTag --lite)") 
 
$THIS_SCRIPT_DIR/to_html.rb --md "${changelog}" > changelog.html

envman add --key CHANGELOG --value "${changelog_final}"

