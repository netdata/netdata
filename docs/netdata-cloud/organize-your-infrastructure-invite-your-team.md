# Spaces and Rooms

This guide explains how to effectively organize your infrastructure monitoring using Netdata Cloud.

Netdata Cloud uses two primary organizational concepts:

- [Spaces](#spaces): High-level containers for your entire infrastructure.
- [Rooms](#rooms): Flexible groupings within Spaces for specific monitoring needs.

## Spaces

**What is a Space?**

Space serves as your primary collaboration environment in Netdata Cloud. It allows you to:

- Organize team members and manage access levels.
- Connect nodes for monitoring.
- Create a unified monitoring environment.

**Key Space Characteristics**

- Each node can only belong to **one** Space.
- You can create multiple Spaces, but we recommend using a single Space for most use cases.
- All team members in a Space can access its monitoring data based on their assigned roles.

### Space Management

**Navigation**

1. Use the left-most sidebar to switch between Spaces.
2. Click the plus (**+**) icon to create a new Space.

**Settings and Configuration**

1. Select your Space.
2. Click the gear icon in the lower left corner.
3. Access settings for:
    - Room management.
    - Node configuration.
    - Integration setup.
    - General Space settings.

## Rooms

**What is a Room?**

Rooms are organizational units within a Space that provide:

- Infrastructure-wide dashboards.
- Real-time metrics visualization.
- Focused monitoring views.
- Flexible node grouping.

**Key Room Characteristics**

- A node can belong to **multiple** Rooms.
- All nodes automatically appear in the "All nodes" Room.
- Each Room has independent dashboards and monitoring tools.

### Room Organization Strategies

1. **Service-Based Organization**

   Group nodes by:
    - Specific services (Nginx, MySQL, Pulsar).
    - Purpose (webserver, database, application).
    - Physical location.
    - Infrastructure type (bare metal, containers).
    - Cloud provider.

2. **End-to-End Application Monitoring**

   Create Rooms for:
    - Complete SaaS product stacks.
    - Internal service dependencies.
    - Full application ecosystems including Kubernetes clusters, Docker containers, Proxies, Databases, Web servers, and Message brokers.

3. **Incident Response**

   Create dedicated Rooms for:
    - Active incident investigation.
    - Problem diagnosis.
    - Performance troubleshooting.
    - Root cause analysis.

### Room Management

**Navigation**

1. Access Rooms through the Space's sidebar.
2. Click the green plus (**+**) icon next to "Rooms" to create new Rooms.

   <img src="https://github.com/user-attachments/assets/16958ba8-53ac-4e78-a51f-7ea328e97f31" height="400px" alt="Individual Space sidebar"/>

**Settings and Configuration**

1. Click the gear icon next to the Room name.
2. Manage:
    - Room access.
    - Node grouping.
    - Dashboard settings.
    - Monitoring configurations.

## Team Collaboration

**Inviting Team Members**

1. Click "Invite Users" in the Space's sidebar.
2. Set appropriate access levels:
    - Rooms.
    - User roles.

**Best Practices for Team Access**

- Invite all relevant team members (SRE, DevOps, ITOps).
- Configure role-based access control.
- Maintain clear permission hierarchies.
- Regular access review and updates.
