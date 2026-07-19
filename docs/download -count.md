Using CLI:
gh api repos/kaanyinaele/routeviewnet/releases/latest \
  --jq '.assets[] | [.name, .download_count] | @tsv'