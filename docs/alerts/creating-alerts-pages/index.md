# 2. Creating and Managing Alerts

This chapter covers how to create, edit, and manage alerts using configuration files or the Netdata Cloud UI.

## What You'll Find in This Chapter

| Section | What It Covers |
|---------|----------------|
| **2.1 Quick Start: Create Your First Alert** | Step-by-step path to modify or add a simple alert |
| **2.2 Creating and Editing Alerts via Configuration Files** | Using health.d, edit-config, and reloading |
| **2.3 Creating and Editing Alerts via Netdata Cloud** | Using the Alerts Configuration Manager |
| **2.4 Managing Stock versus Custom Alerts** | Overriding stock alerts safely |
| **2.5 Reloading and Validating Alert Configuration** | Applying changes and confirming load |

## How to Navigate This Chapter

- Start at **2.1** if you want a quick test alert
- Go to **2.2** for file-based editing
- Use **2.3** for Cloud UI configuration
- See **2.4** before modifying stock alerts
- Jump to **2.5** after making configuration changes

## Key Concepts

- Alerts can be created via **configuration files** or **Cloud UI**
- Stock alerts are in `/usr/lib/netdata/conf.d/health.d/`
- Custom alerts go in `/etc/netdata/health.d/`
- Cloud-defined alerts coexist with file-based ones

## What's Next

- **Chapter 3**: Alert Configuration Syntax (detailed reference)
- **Chapter 4**: Controlling Alerts and Noise

See also: [Alert Configuration Syntax](../alert-configuration-syntax/alert-configuration-syntax.md)