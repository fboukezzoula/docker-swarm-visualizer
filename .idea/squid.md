# Quick debug test on ONE subscription
az graph query -q "
resources
| where type in ('Microsoft.Storage/storageAccounts', 'Microsoft.KeyVault/vaults')
| project id, name, type, properties
| limit 10
" --subscriptions "YOUR-SUB-ID" -o json | jq '.[].type' | sort -u
