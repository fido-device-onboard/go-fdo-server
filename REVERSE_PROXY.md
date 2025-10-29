# FDO Management API Reverse Proxy Setup

This document provides instructions for setting up nginx or Apache as a reverse proxy with basic authentication for FDO management APIs.

## Prerequisites

Before setting up the reverse proxy, ensure you have:

1. **Running FDO Services**: At least one of the following FDO servers must be running:
   - Manufacturing server (typically on port 8038)
   - Owner server (typically on port 8043)
   - Rendezvous server (typically on port 8041, mainly for FDO protocol)

2. **Domain Names**: Dedicated domain names or subdomains for each service you want to proxy:
   - `fdo-mfg.example.com` for Manufacturing APIs
   - `fdo-owner.example.com` for Owner APIs

3. **TLS Certificates**: Valid TLS certificates for your domain names (self-signed certificates can be used for testing, but production deployments should use certificates from a trusted CA)

4. **Network Access**: The reverse proxy server must be able to connect to the FDO services on their configured ports

## Overview

The FDO server exposes management APIs on `/api/v1/` endpoints without built-in authentication. A reverse proxy can provide:

- Basic HTTP authentication
- TLS termination
- Access control and security headers
- Rate limiting (additional protection beyond built-in limits)

## Management API Protection

The FDO server exposes management APIs under the `/api/v1/` path prefix that require protection:

- **Manufacturing Service** (typically port 8038): Management APIs for rendezvous info and vouchers
- **Owner Service** (typically port 8043): Management APIs for device ownership and onboarding

The reverse proxy should:
- **Require authentication** for all `/api/v1/*` requests  
- **Allow unauthenticated access** to `/health` and `/fdo/101/msg/*` endpoints

## Common Setup Steps

### 1. Install Required Packages

```bash
# For nginx
sudo dnf install nginx httpd-tools
sudo systemctl enable nginx

# For Apache  
sudo dnf install httpd mod_ssl httpd-tools
sudo systemctl enable httpd
```

### 2. Create Password File

```bash
# For nginx
sudo htpasswd -c /etc/nginx/fdo-mgmt.passwd admin

# For Apache  
sudo htpasswd -c /etc/httpd/fdo-mgmt.passwd admin
# Enter password when prompted
```

**Important notes:**
- Record the password you enter, as it will be hashed in the password file and cannot be recovered. You'll need this password for API authentication.
- The `-c` flag creates a new password file, overwriting any existing file. For adding additional users later, omit the `-c` flag: `sudo htpasswd /etc/nginx/fdo-mgmt.passwd another_user`

### 3. Firewall Configuration

```bash
# Allow HTTPS traffic (required for both)
sudo firewall-cmd --add-service=https --permanent
sudo firewall-cmd --reload
```

## Nginx Configuration

### Configuration File

The nginx configuration below creates two separate virtual hosts (one for Manufacturing APIs, one for Owner APIs) with the following security features:

- **TLS Encryption**: Forces HTTPS with modern TLS protocols and secure ciphers
- **Basic Authentication**: Requires username/password for `/api/v1/` endpoints  
- **Security Headers**: Adds headers to protect against common attacks
- **Health Check Access**: Allows unauthenticated access to `/health` for monitoring
- **Request Blocking**: Returns 404 for any other requests

Create `/etc/nginx/conf.d/fdo-mgmt.conf`:

```nginx
# FDO Manufacturing Management API
server {
    listen 443 ssl http2;
    server_name fdo-mfg.example.com;
    
    # TLS Configuration
    ssl_certificate /path/to/your/cert.pem;
    ssl_certificate_key /path/to/your/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    
    # Security Headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    
    # Protect management APIs
    location /api/v1/ {
        auth_basic "FDO Management";
        auth_basic_user_file /etc/nginx/fdo-mgmt.passwd;
        
        proxy_pass http://127.0.0.1:8038;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts for API calls
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }
    
    # Health check (no auth required)
    location /health {
        proxy_pass http://127.0.0.1:8038;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # FDO protocol endpoints (no auth required)
    location /fdo/101/ {
        proxy_pass http://127.0.0.1:8038;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts for protocol calls
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }
    
    # Block everything else
    location / {
        return 404;
    }
}

# FDO Owner Management API
server {
    listen 443 ssl http2;
    server_name fdo-owner.example.com;
    
    # TLS Configuration (same as above)
    ssl_certificate /path/to/your/cert.pem;
    ssl_certificate_key /path/to/your/key.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    
    # Security Headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    
    # Protect management APIs
    location /api/v1/ {
        auth_basic "FDO Management";
        auth_basic_user_file /etc/nginx/fdo-mgmt.passwd;
        
        proxy_pass http://127.0.0.1:8043;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts for API calls
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }
    
    # Health check (no auth required)
    location /health {
        proxy_pass http://127.0.0.1:8043;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # FDO protocol endpoints (no auth required)
    location /fdo/101/ {
        proxy_pass http://127.0.0.1:8043;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts for protocol calls
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
    }
    
    # Block everything else
    location / {
        return 404;
    }
}

# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name fdo-mfg.example.com fdo-owner.example.com;
    return 301 https://$server_name$request_uri;
}
```

### Start and Test nginx

```bash
# Start nginx
sudo systemctl start nginx

# Test configuration syntax
sudo nginx -t

# Reload nginx to apply the new configuration (reload is safer than restart as it maintains existing connections)
sudo systemctl reload nginx
```

## Apache Configuration

### Configuration File

The Apache configuration provides equivalent functionality to the nginx setup using Apache's module system:

- **Module Loading**: Explicitly loads required proxy and authentication modules
- **Virtual Hosts**: Separate virtual hosts for Manufacturing and Owner services
- **Security**: Same TLS configuration and security headers as nginx
- **Access Control**: Uses Apache's location-based access control for authentication

Create `/etc/httpd/conf.d/fdo-mgmt.conf`:

```apache
# Note: Required modules (proxy, auth_basic, authn_file, authz_user) are typically 
# enabled by default on RHEL/CentOS/Fedora. If they are not enabled, you may need to
# edit files in /etc/httpd/conf.modules.d/ to load them.

# FDO Manufacturing Management API
<VirtualHost *:443>
    ServerName fdo-mfg.example.com
    
    # TLS Configuration
    SSLEngine on
    SSLCertificateFile /path/to/your/cert.pem
    SSLCertificateKeyFile /path/to/your/key.pem
    SSLProtocol all -SSLv3 -TLSv1 -TLSv1.1
    SSLCipherSuite ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    SSLHonorCipherOrder off
    
    # Security Headers
    Header always set X-Frame-Options DENY
    Header always set X-Content-Type-Options nosniff
    Header always set Strict-Transport-Security "max-age=31536000; includeSubDomains"
    
    # Block everything else by default
    <Location "/">
        Require all denied
    </Location>

    # Protect management APIs
    <Location "/api/v1/">
        AuthType Basic
        AuthName "FDO Management"
        AuthUserFile /etc/httpd/fdo-mgmt.passwd
        Require valid-user
        
        ProxyPreserveHost On
        ProxyPass http://127.0.0.1:8038/api/v1/
        ProxyPassReverse http://127.0.0.1:8038/api/v1/
        
    </Location>
    
    # Health check (no auth required)
    <Location "/health">
        Require all granted
        ProxyPreserveHost On
        ProxyPass http://127.0.0.1:8038/health
        ProxyPassReverse http://127.0.0.1:8038/health
    </Location>
    
    # FDO protocol endpoints (no auth required)
    <Location "/fdo/101/">
        Require all granted
        ProxyPreserveHost On
        ProxyPass http://127.0.0.1:8038/fdo/101/
        ProxyPassReverse http://127.0.0.1:8038/fdo/101/
    </Location>
</VirtualHost>

# FDO Owner Management API
<VirtualHost *:443>
    ServerName fdo-owner.example.com
    
    # TLS Configuration (same as above)
    SSLEngine on
    SSLCertificateFile /path/to/your/cert.pem
    SSLCertificateKeyFile /path/to/your/key.pem
    SSLProtocol all -SSLv3 -TLSv1 -TLSv1.1
    SSLCipherSuite ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    SSLHonorCipherOrder off
    
    # Security Headers
    Header always set X-Frame-Options DENY
    Header always set X-Content-Type-Options nosniff
    Header always set Strict-Transport-Security "max-age=31536000; includeSubDomains"
    
    # Block everything else by default
    <Location "/">
        Require all denied
    </Location>

    # Protect management APIs
    <Location "/api/v1/">
        AuthType Basic
        AuthName "FDO Management"
        AuthUserFile /etc/httpd/fdo-mgmt.passwd
        Require valid-user
        
        ProxyPreserveHost On
        ProxyPass http://127.0.0.1:8043/api/v1/
        ProxyPassReverse http://127.0.0.1:8043/api/v1/
        
    </Location>
    
    # Health check (no auth required)
    <Location "/health">
        Require all granted
        ProxyPreserveHost On
        ProxyPass http://127.0.0.1:8043/health
        ProxyPassReverse http://127.0.0.1:8043/health
    </Location>
    
    # FDO protocol endpoints (no auth required)
    <Location "/fdo/101/">
        Require all granted
        ProxyPreserveHost On
        ProxyPass http://127.0.0.1:8043/fdo/101/
        ProxyPassReverse http://127.0.0.1:8043/fdo/101/
    </Location>
</VirtualHost>

# Redirect HTTP to HTTPS
<VirtualHost *:80>
    ServerName fdo-mfg.example.com
    ServerAlias fdo-owner.example.com
    Redirect permanent / https://%{HTTP_HOST}/
</VirtualHost>
```

### Start and Test Apache

```bash
# Start Apache
sudo systemctl start httpd

# Test configuration syntax
sudo httpd -t

# Restart Apache to apply the new configuration (Apache requires restart rather than reload for new virtual hosts)
sudo systemctl restart httpd
```

## Testing the Setup

The examples below assume you have valid TLS certificates from a trusted Certificate Authority. If you're using self-signed certificates for testing, see the "Self-Signed Certificate Testing" section below.

### 1. Test Without Authentication (should fail)

```bash
curl -i https://fdo-mfg.example.com/api/v1/rvinfo
# Expected: 401 Unauthorized
```

### 2. Test With Authentication

```bash
curl -i -u admin:password https://fdo-mfg.example.com/api/v1/rvinfo
# Expected: 200 OK with RV info data

curl -i -u admin:password https://fdo-owner.example.com/api/v1/owner/redirect
# Expected: 200 OK with redirect data
```

### 3. Test Health Endpoint (no auth required)

```bash
curl -i https://fdo-mfg.example.com/health
# Expected: 200 OK

curl -i https://fdo-owner.example.com/health
# Expected: 200 OK
```

### Self-Signed Certificate Testing

If you're using self-signed certificates for testing, curl will reject the connection by default. Use the `-k` flag to skip certificate verification:

```bash
# Test without auth (should fail with 401)
curl -i -k https://fdo-mfg.example.com/api/v1/rvinfo

# Test with auth (should succeed)
curl -i -k -u admin:password https://fdo-mfg.example.com/api/v1/rvinfo

# Test health endpoint
curl -i -k https://fdo-mfg.example.com/health

# Test Owner service endpoints
curl -i -k -u admin:password https://fdo-owner.example.com/api/v1/owner/redirect
curl -i -k https://fdo-owner.example.com/health
```

**Note**: The `-k` flag disables certificate verification and should only be used for development and testing. Production deployments should use certificates from a trusted CA or Let's Encrypt.

## Important Notes

1. **FDO Protocol Endpoints**: The reverse proxy only protects management APIs (`/api/v1/`). The FDO protocol endpoints (`/fdo/101/msg/`) should remain accessible for device communication.

2. **Certificate Management**: Replace `/path/to/your/cert.pem` and `/path/to/your/key.pem` with actual certificate paths. Consider using Let's Encrypt for free certificates.

3. **Password Security**: Use strong passwords and consider implementing additional security measures like IP whitelisting.

4. **Monitoring**: Enable access logs to monitor API usage and potential security threats.

5. **Rate Limiting**: The FDO server includes built-in rate limiting (2 requests/second, burst of 10). The reverse proxy can add additional protection if needed.

For RHEL-specific documentation on reverse proxy setups, refer to:
- [RHEL System Administrator's Guide - HTTP Servers](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/9/html/deploying_different_types_of_servers/setting-apache-http-server_deploying-different-types-of-servers)
- [RHEL Security Guide - TLS Configuration](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/9/html/security_hardening/)