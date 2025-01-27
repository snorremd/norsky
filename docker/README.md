# Docker Deployment

This directory contains Docker Compose configurations for both development and production deployments of Norsky.

## Files Overview

- `dev-compose.yml` - Development environment configuration with PostgreSQL and pgAdmin exposed on localhost
- `compose.yml` - Simplest production configuration with PostgreSQL, pgAdmin and Norsky
- `compose.traefik.yml` - Production configuration with Traefik for HTTPS and automatic TLS certificates as well as PostgreSQL, pgAdmin and Norsky
- `.env.example` - Example environment variables file

## Quick Start

Copy the `.env.example` file to `.env` and add secure passwords for PostgreSQL and pgAdmin.
Choose which compose file you want to use, The one with Traefik is a nice production setup if you don't alread run a reverse proxy.

Now run the compose file (as root if necessary):

```bash
cd docker
docker compose up -d
```
You can edit the version of PostgreSQL and pgAdmin in the compose file if you want to use a later version.

## Configuration

### Production Deployment

The production configuration (`compose.yml`) includes:

- PostgreSQL with optimized settings
- PgAdmin for database management
- Automated backups
- Resource limits and reservations
- Local-only port exposure for security

### Development Environment

The development configuration (`dev-compose.yml`) includes:

- PostgreSQL with default settings
- PgAdmin for database management
- Simplified configuration for local development

## Environment Variables

Required environment variables:

- `POSTGRES_DB`: Database name (default: norsky)
- `POSTGRES_USER`: Database user (default: norsky)
- `POSTGRES_PASSWORD`: Database password (required)
- `PGADMIN_DEFAULT_EMAIL`: PgAdmin login email
- `PGADMIN_DEFAULT_PASSWORD`: PgAdmin login password

## Security Notes

- Production configuration only exposes ports locally (127.0.0.1)
- Use strong passwords in the .env file
- Keep your .env file secure and never commit it to version control 