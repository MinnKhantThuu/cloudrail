#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
task_root="$PWD"
task_go="${CLOUDRAIL_GO:-go}"
task_stage="$task_root/.data/release-stage"
mkdir -p "$task_stage"
npm --prefix apps/web ci
npm --prefix apps/web run build
for task_arch in amd64 arm64; do
  task_target="$task_stage/linux-$task_arch"
  mkdir -p "$task_target"
  for task_binary in server agent; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$task_arch" "$task_go" build -buildvcs=false -trimpath -ldflags='-s -w -buildid=' -o "$task_target/$task_binary" "./cmd/$task_binary"
  done
  rm -rf "$task_target/web" "$task_target/notices"
  cp -R apps/web/dist "$task_target/web"
done
"$task_go" list -m -json all > "$task_stage/modules.json"
python3 - "$task_stage" "$("$task_go" env GOROOT)" <<'PY'
import json,pathlib,shutil,sys
stage=pathlib.Path(sys.argv[1]);root=pathlib.Path.cwd()
notices=stage/'linux-amd64/notices';notices.mkdir(parents=True,exist_ok=True)
shutil.copyfile(pathlib.Path(sys.argv[2])/'LICENSE',notices/'Go-LICENSE')
for f in (root/'apps/web/node_modules').rglob('*'):
    if f.is_file() and not f.is_symlink() and f.name.upper().startswith(('LICENSE','LICENCE','NOTICE','COPYING','COPYRIGHT')):
        target=notices/'npm'/f.relative_to(root/'apps/web/node_modules');target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(f,target)
text=(stage/'modules.json').read_text();decoder=json.JSONDecoder()
while text.strip():
    obj,end=decoder.raw_decode(text.lstrip());text=text.lstrip()[end:]
    if obj.get('Main') or not obj.get('Dir'):continue
    for f in pathlib.Path(obj['Dir']).iterdir():
        if f.is_file() and f.name.upper().startswith(('LICENSE','NOTICE','COPYING','COPYRIGHT','PATENTS')):
            target=notices/'go'/(obj['Path']+'@'+obj['Version'])/f.name;target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(f,target)
shutil.copytree(notices,stage/'linux-arm64/notices')
PY
python3 scripts/package-release.py --runtime "$task_stage"
