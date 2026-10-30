#!/usr/bin/env bash
# Gera PNGs a partir de arquivos PlantUML usando a imagem Docker plantuml/plantuml.
#
# Convenção: o código fica em <pasta>/plantuml/<nome>.puml e a imagem é salva
# em <pasta>/<nome>.png (a pasta acima de plantuml/).
#
# Uso (Git Bash, de qualquer diretório):
#   ./generate.sh caminho/para/diagrama.puml [outro.puml ...]
#   ./generate.sh            # sem argumentos: todos os .puml da pasta deste script
set -euo pipefail

if [ "$#" -eq 0 ]; then
  script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  shopt -s nullglob
  set -- "$script_dir"/*.puml
  if [ "$#" -eq 0 ]; then
    echo "Nenhum .puml encontrado em $script_dir" >&2
    exit 1
  fi
fi

for f in "$@"; do
  if [ ! -f "$f" ]; then
    echo "Arquivo não encontrado: $f" >&2
    exit 1
  fi

  src_dir=$(cd "$(dirname "$f")" && pwd)
  out_dir=$(dirname "$src_dir")
  src_name=$(basename "$src_dir")
  file=$(basename "$f")

  MSYS_NO_PATHCONV=1 docker run --rm \
    -v "$(cygpath -w "$out_dir"):/data" \
    -w "/data/$src_name" \
    plantuml/plantuml -charset UTF-8 -tpng -o /data "$file"

  echo "Gerado: $out_dir/${file%.puml}.png"
done
