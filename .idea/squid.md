# Azure Multi-Tenant Endpoint Export Script

Here's the complete script in English with your specific Azure services:

```bash
#!/bin/bash
#
# Azure Multi-Tenant Endpoint Export
# Lists all Azure resources endpoints across all tenants and subscriptions
# Outputs CSV files filtered by endpoint pattern
#

set -euo pipefail

# ============================================
# CONFIGURATION
# ============================================

# Azure endpoints to whitelist (will create one CSV per endpoint)
ENDPOINTS=(
    "blob.core.windows.net"
    "file.core.windows.net"
    "queue.core.windows.net"
    "table.core.windows.net"
    "hcp.westeurope.azmk8s.io"
    "northeurope.azmk8s.io"
    "vault.azure.net"
    "servicebus.windows.net"
    "azureedge.net"
    "azurewebsites.net"
    "graph.microsoft.com"
    "graph.windows.net"
)

# Output directory
OUTPUT_DIR="azure_endpoints_export"

# Parallel jobs count
PARALLEL_JOBS=20

# ============================================
# FUNCTIONS
# ============================================

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# Check Azure CLI installation and authentication
check_az_cli() {
    if ! command -v az &> /dev/null; then
        error "Azure CLI is not installed"
        exit 1
    fi
    
    if ! az account show &> /dev/null; then
        error "Not logged in to Azure. Run: az login"
        exit 1
    fi
    
    log "Azure CLI check passed"
}

# Initialize CSV files with headers
init_csv_files() {
    log "Initializing CSV files..."
    for endpoint in "${ENDPOINTS[@]}"; do
        local filename=$(echo "$endpoint" | sed 's/\*/wildcard/g' | tr '.' '_')
        echo "tenantid,subscriptionid,fqdn" > "$OUTPUT_DIR/${endpoint}.csv"
    done
}

# Extract FQDN based on resource type
get_fqdn_for_type() {
    local type=$1
    local props=$2
    local fqdn=""
    
    case "$type" in
        "Microsoft.Storage/storageAccounts")
            # Check all endpoint types (blob, file, queue, table, dfs)
            fqdn=$(echo "$props" | jq -r '
                (.primaryEndpoints.blob // .primaryEndpoints.file // .primaryEndpoints.queue // .primaryEndpoints.table // .primaryEndpoints.dfs // "") 
                | if . != "" then gsub("https://"; "") | gsub("/"; "") else "" end
            ')
            ;;
        "Microsoft.KeyVault/vaults")
            fqdn=$(echo "$props" | jq -r '.vaultUri // ""' | sed 's|https://||;s|/||')
            ;;
        "Microsoft.ContainerService/managedClusters")
            fqdn=$(echo "$props" | jq -r '.fqdn // .agentPoolProfiles[].fqdn // ""')
            ;;
        "Microsoft.ServiceBus/namespaces")
            fqdn=$(echo "$props" | jq -r '.serviceBusEndpoint // ""' | sed 's|https://||;s|/||')
            ;;
        "Microsoft.Web/sites"|"Microsoft.Web/staticsites")
            fqdn=$(echo "$props" | jq -r '.defaultHostName // .hostName // ""')
            ;;
        "Microsoft.PowerPlatform/accounts")
            fqdn=$(echo "$props" | jq -r '.domain // ""')
            ;;
        "Microsoft.AzurePlaywrightTest/accounts")
            fqdn=$(echo "$props" | jq -r '.fqdn // ""')
            ;;
        *)
            fqdn=""
            ;;
    esac
    
    echo "$fqdn"
}

export -f get_fqdn_for_type

# Export subscription resources to CSV
export_subscription() {
    local tenant_id=$1
    local sub_id=$2
    
    # Set the subscription context
    az account set --subscription "$sub_id" 2>/dev/null || return
    
    # Query Azure Resource Graph for all relevant resource types
    local result
    result=$(az graph query -q "
        resources
        | where type in (
            'Microsoft.Storage/storageAccounts',
            'Microsoft.KeyVault/vaults',
            'Microsoft.ContainerService/managedClusters',
            'Microsoft.ServiceBus/namespaces',
            'Microsoft.Web/sites',
            'Microsoft.Web/staticsites',
            'Microsoft.PowerPlatform/accounts',
            'Microsoft.AzurePlaywrightTest/accounts'
        )
        | project id, name, type, properties
        | limit 10000
    " --subscriptions "$sub_id" -o json 2>/dev/null || echo "[]")
    
    # Process each resource
    echo "$result" | jq -r --arg tenant "$tenant_id" \
        '.[] | "\($tenant)|\(.id)|\(.name)|\(.type)|\(.properties | tojson)"' | \
    while IFS='|' read -r t_id res_id name type props; do
        # Get FQDN(s) for this resource type
        fqdns=$(get_fqdn_for_type "$type" "$props")
        
        # Handle multiple FQDNs (some resources have multiple endpoints)
        for fqdn in $fqdns; do
            if [[ -n "$fqdn" && "$fqdn" != "null" && "$fqdn" != "" ]]; then
                # Check against all whitelisted endpoints
                for endpoint in "${ENDPOINTS[@]}"; do
                    if echo "$fqdn" | grep -qi "$endpoint"; then
                        echo "${t_id},${sub_id},${fqdn}" >> "$OUTPUT_DIR/${endpoint}.csv"
                    fi
                done
            fi
        done
    done
}

export -f export_subscription

# ============================================
# MAIN
# ============================================

main() {
    log "========================================"
    log "Azure Multi-Tenant Endpoint Export"
    log "========================================"
    
    # Create output directory
    mkdir -p "$OUTPUT_DIR"
    
    # Check Azure CLI
    check_az_cli
    
    # Initialize CSV files
    init_csv_files
    
    # Get all unique tenants
    log "Retrieving tenant list..."
    local tenants
    tenants=$(az account list -o json | jq -r '.[].tenantId' | sort -u)
    
    if [[ -z "$tenants" ]]; then
        error "No tenants found"
        exit 1
    fi
    
    local tenant_count
    tenant_count=$(echo "$tenants" | wc -l)
    log "Found $tenant_count tenant(s)"
    
    # Build list of all subscriptions from all tenants
    log "Retrieving subscriptions from all tenants..."
    
    local all_subs=""
    for tenant in $tenants; do
        log "  Processing tenant: $tenant"
        local subs
        subs=$(az account list --tenant "$tenant" -o json 2>/dev/null | \
            jq -r --arg t "$tenant" '.[] | "\($t)|\(.id)"' 2>/dev/null || true)
        all_subs+="$subs"$'\n'
    done
    
    local sub_count
    sub_count=$(echo "$all_subs" | grep -c '^' || echo 0)
    log "Found $sub_count subscription(s) to process"
    
    if [[ $sub_count -eq 0 ]]; then
        error "No subscriptions found"
        exit 1
    fi
    
    # Parallel execution
    log "Starting parallel export ($PARALLEL_JOBS parallel jobs)..."
    echo "$all_subs" | grep -v '^$' | xargs -P $PARALLEL_JOBS -I {} bash -c 'export_subscription "$@"' _ {}
    
    # Remove duplicates
    log "Removing duplicates..."
    for endpoint in "${ENDPOINTS[@]}"; do
        if [[ -f "$OUTPUT_DIR/${endpoint}.csv" ]]; then
            {
                head -1 "$OUTPUT_DIR/${endpoint}.csv"
                tail -n +2 "$OUTPUT_DIR/${endpoint}.csv" | sort -u
            } > "$OUTPUT_DIR/${endpoint}.tmp" && mv "$OUTPUT_DIR/${endpoint}.tmp" "$OUTPUT_DIR/${endpoint}.csv"
        fi
    done
    
    # Summary
    log ""
    log "========================================"
    log "EXPORT COMPLETE"
    log "========================================"
    log "Output directory: $OUTPUT_DIR/"
    log ""
    log "Results:"
    
    for endpoint in "${ENDPOINTS[@]}"; do
        if [[ -f "$OUTPUT_DIR/${endpoint}.csv" ]]; then
            local count
            count=$(($(wc -l < "$OUTPUT_DIR/${endpoint}.csv}") - 1))
            if [[ $count -gt 0 ]]; then
                log "  $endpoint.csv : $count entries"
            else
                log "  $endpoint.csv : 0 entries"
            fi
        fi
    done
    
    log ""
    log "Done!"
}

# Run main function
main "$@"
```

## Save and Execute

```bash
# Save the script
cat > azure_export.sh << 'SCRIPT_EOF'
#!/bin/bash
#
# Azure Multi-Tenant Endpoint Export
# Lists all Azure resources endpoints across all tenants and subscriptions
# Outputs CSV files filtered by endpoint pattern
#

set -euo pipefail

# ============================================
# CONFIGURATION
# ============================================

# Azure endpoints to whitelist (will create one CSV per endpoint)
ENDPOINTS=(
    "blob.core.windows.net"
    "file.core.windows.net"
    "queue.core.windows.net"
    "table.core.windows.net"
    "hcp.westeurope.azmk8s.io"
    "northeurope.azmk8s.io"
    "vault.azure.net"
    "servicebus.windows.net"
    "azureedge.net"
    "azurewebsites.net"
    "graph.microsoft.com"
    "graph.windows.net"
)

# Output directory
OUTPUT_DIR="azure_endpoints_export"

# Parallel jobs count
PARALLEL_JOBS=20

# ============================================
# FUNCTIONS
# ============================================

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
}

error() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*" >&2
}

# Check Azure CLI installation and authentication
check_az_cli() {
    if ! command -v az &> /dev/null; then
        error "Azure CLI is not installed"
        exit 1
    fi
    
    if ! az account show &> /dev/null; then
        error "Not logged in to Azure. Run: az login"
        exit 1
    fi
    
    log "Azure CLI check passed"
}

# Initialize CSV files with headers
init_csv_files() {
    log "Initializing CSV files..."
    for endpoint in "${ENDPOINTS[@]}"; do
        echo "tenantid,subscriptionid,fqdn" > "$OUTPUT_DIR/${endpoint}.csv"
    done
}

# Extract FQDN based on resource type
get_fqdn_for_type() {
    local type=$1
    local props=$2
    local fqdn=""
    
    case "$type" in
        "Microsoft.Storage/storageAccounts")
            fqdn=$(echo "$props" | jq -r '
                (.primaryEndpoints.blob // .primaryEndpoints.file // .primaryEndpoints.queue // .primaryEndpoints.table // .primaryEndpoints.dfs // "") 
                | if . != "" then gsub("https://"; "") | gsub("/"; "") else "" end
            ')
            ;;
        "Microsoft.KeyVault/vaults")
            fqdn=$(echo "$props" | jq -r '.vaultUri // ""' | sed 's|https://||;s|/||')
            ;;
        "Microsoft.ContainerService/managedClusters")
            fqdn=$(echo "$props" | jq -r '.fqdn // .agentPoolProfiles[].fqdn // ""')
            ;;
        "Microsoft.ServiceBus/namespaces")
            fqdn=$(echo "$props" | jq -r '.serviceBusEndpoint // ""' | sed 's|https://||;s|/||')
            ;;
        "Microsoft.Web/sites"|"Microsoft.Web/staticsites")
            fqdn=$(echo "$props" | jq -r '.defaultHostName // .hostName // ""')
            ;;
        "Microsoft.PowerPlatform/accounts")
            fqdn=$(echo "$props" | jq -r '.domain // ""')
            ;;
        "Microsoft.AzurePlaywrightTest/accounts")
            fqdn=$(echo "$props" | jq -r '.fqdn // ""')
            ;;
        *)
            fqdn=""
            ;;
    esac
    
    echo "$fqdn"
}

export -f get_fqdn_for_type

# Export subscription resources to CSV
export_subscription() {
    local tenant_id=$1
    local sub_id=$2
    
    # Set the subscription context
    az account set --subscription "$sub_id" 2>/dev/null || return
    
    # Query Azure Resource Graph for all relevant resource types
    local result
    result=$(az graph query -q "
        resources
        | where type in (
            'Microsoft.Storage/storageAccounts',
            'Microsoft.KeyVault/vaults',
            'Microsoft.ContainerService/managedClusters',
            'Microsoft.ServiceBus/namespaces',
            'Microsoft.Web/sites',
            'Microsoft.Web/staticsites',
            'Microsoft.PowerPlatform/accounts',
            'Microsoft.AzurePlaywrightTest/accounts'
        )
        | project id, name, type, properties
        | limit 10000
    " --subscriptions "$sub_id" -o json 2>/dev/null || echo "[]")
    
    # Process each resource
    echo "$result" | jq -r --arg tenant "$tenant_id" \
        '.[] | "\($tenant)|\(.id)|\(.name)|\(.type)|\(.properties | tojson)"' | \
    while IFS='|' read -r t_id res_id name type props; do
        # Get FQDN(s) for this resource type
        fqdns=$(get_fqdn_for_type "$type" "$props")
        
        # Handle multiple FQDNs (some resources have multiple endpoints)
        for fqdn in $fqdns; do
            if [[ -n "$fqdn" && "$fqdn" != "null" && "$fqdn" != "" ]]; then
                # Check against all whitelisted endpoints
                for endpoint in "${ENDPOINTS[@]}"; do
                    if echo "$fqdn" | grep -qi "$endpoint"; then
                        echo "${t_id},${sub_id},${fqdn}" >> "$OUTPUT_DIR/${endpoint}.csv"
                    fi
                done
            fi
        done
    done
}

export -f export_subscription

# ============================================
# MAIN
# ============================================

main() {
    log "========================================"
    log "Azure Multi-Tenant Endpoint Export"
    log "========================================"
    
    # Create output directory
    mkdir -p "$OUTPUT_DIR"
    
    # Check Azure CLI
    check_az_cli
    
    # Initialize CSV files
    init_csv_files
    
    # Get all unique tenants
    log "Retrieving tenant list..."
    local tenants
    tenants=$(az account list -o json | jq -r '.[].tenantId' | sort -u)
    
    if [[ -z "$tenants" ]]; then
        error "No tenants found"
        exit 1
    fi
    
    local tenant_count
    tenant_count=$(echo "$tenants" | wc -l)
    log "Found $tenant_count tenant(s)"
    
    # Build list of all subscriptions from all tenants
    log "Retrieving subscriptions from all tenants..."
    
    local all_subs=""
    for tenant in $tenants; do
        log "  Processing tenant: $tenant"
        local subs
        subs=$(az account list --tenant "$tenant" -o json 2>/dev/null | \
            jq -r --arg t "$tenant" '.[] | "\($t)|\(.id)"' 2>/dev/null || true)
        all_subs+="$subs"$'\n'
    done
    
    local sub_count
    sub_count=$(echo "$all_subs" | grep -c '^' || echo 0)
    log "Found $sub_count subscription(s) to process"
    
    if [[ $sub_count -eq 0 ]]; then
        error "No subscriptions found"
        exit 1
    fi
    
    # Parallel execution
    log "Starting parallel export ($PARALLEL_JOBS parallel jobs)..."
    echo "$all_subs" | grep -v '^$' | xargs -P $PARALLEL_JOBS -I {} bash -c 'export_subscription "$@"' _ {}
    
    # Remove duplicates
    log "Removing duplicates..."
    for endpoint in "${ENDPOINTS[@]}"; do
        if [[ -f "$OUTPUT_DIR/${endpoint}.csv" ]]; then
            {
                head -1 "$OUTPUT_DIR/${endpoint}.csv"
                tail -n +2 "$OUTPUT_DIR/${endpoint}.csv" | sort -u
            } > "$OUTPUT_DIR/${endpoint}.tmp" && mv "$OUTPUT_DIR/${endpoint}.tmp" "$OUTPUT_DIR/${endpoint}.csv"
        fi
    done
    
    # Summary
    log ""
    log "========================================"
    log "EXPORT COMPLETE"
    log "========================================"
    log "Output directory: $OUTPUT_DIR/"
    log ""
    log "Results:"
    
    for endpoint in "${ENDPOINTS[@]}"; do
        if [[ -f "$OUTPUT_DIR/${endpoint}.csv" ]]; then
            local count
            count=$(($(wc -l < "$OUTPUT_DIR/${endpoint}.csv}") - 1))
            if [[ $count -gt 0 ]]; then
                log "  $endpoint.csv : $count entries"
            else
                log "  $endpoint.csv : 0 entries"
            fi
        fi
    done
    
    log ""
    log "Done!"
}

# Run main function
main "$@"
SCRIPT_EOF

# Make executable
chmod +x azure_export.sh
```

## Run the Script

```bash
# Make sure you're logged in
az login

# Run the export
./azure_export.sh
```

## Expected Output

```
azure_endpoints_export/
├── blob.core.windows.net.csv      # Storage accounts - blob endpoints
├── file.core.windows.net.csv       # Storage accounts - file endpoints
├── queue.core.windows.net.csv      # Storage accounts - queue endpoints
├── table.core.windows.net.csv      # Storage accounts - table endpoints
├── hcp.westeurope.azmk8s.io.csv    # AKS clusters in West Europe
├── northeurope.azmk8s.io.csv       # AKS clusters in North Europe
├── vault.azure.net.csv             # Key Vaults
├── servicebus.windows.net.csv      # Service Bus namespaces
├── azureedge.net.csv               # Azure Front Door/CDN endpoints
├── azurewebsites.net.csv           # App Services
├── graph.microsoft.com.csv         # Microsoft Graph (modern)
└── graph.windows.net.csv           # Azure AD Graph (legacy)
```

## Azure Services Coverage

| Endpoint Pattern | Azure Service | Resource Type |
|------------------|---------------|---------------|
| `*.blob.core.windows.net` | Azure Storage - Blob | Microsoft.Storage/storageAccounts |
| `*.file.core.windows.net` | Azure Storage - File | Microsoft.Storage/storageAccounts |
| `*.queue.core.windows.net` | Azure Storage - Queue | Microsoft.Storage/storageAccounts |
| `*.table.core.windows.net` | Azure Storage - Table | Microsoft.Storage/storageAccounts |
| `*.azmk8s.io` | Azure Kubernetes Service | Microsoft.ContainerService/managedClusters |
| `*.vault.azure.net` | Azure Key Vault | Microsoft.KeyVault/vaults |
| `*.servicebus.windows.net` | Azure Service Bus | Microsoft.ServiceBus/namespaces |
| `*.azureedge.net` | Azure Front Door / CDN | Microsoft.Cdn/profiles |
| `*.azurewebsites.net` | Azure App Service | Microsoft.Web/sites |
| `*.graph.microsoft.com` | Microsoft Graph | Microsoft.PowerPlatform/accounts |
| `*.graph.windows.net` | Azure AD Graph | Legacy |

Let me know if you need to add more endpoints or have any questions! 🚀
