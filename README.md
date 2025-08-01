# ResolveX

**ResolveX** is a unified solution that integrates BGP and DNS functionalities, allowing users to manage domain names and their associated IP addresses through an intuitive admin panel. The system handles domain name resolution and routes traffic efficiently using BGP.

## Components

ResolveX consists of the following components:

- **API Service**: API and builtin UI for managing domains that will be handled by the service.
- **BGP Service**: Manages IP address announcements via BGP for all resolved domains and subdomains.
- **Broadcaster**: Used to communicate with BGP Peers: add / remove peer, send updates to peer.
- **Domain Util**: Allow validating domain name and fetches a list of domain names from http-link.  
- **DNS Service**: Resolves domain names and subdomains, using multiple external DNS servers.
- **Store**: Stores domain and ip address, invalidates cache and notifies BGP peers about updates.

## Use Cases

ResolveX supports the following key use cases:

1. **Adding Domains**:  
   Users can add domains through the admin panel. The system supports adding single domains.  
   TODO:
     - for domains with wildcards (regexp), ResolveX should listen for DNS queries, progressively resolving and adding subdomains to the BGP announcements.

2. **Handling Subdomains**:  
   For domains that include subdomains, incoming DNS requests are monitored. As new subdomains are detected, they are resolved and added to the BGP announcements automatically.

3. **BGP Announcements**:  
   Once a domain is added, ResolveX resolves its IP addresses and announces them via BGP, ensuring efficient routing for all requests.

4. **External DNS Resolution**:  
   ResolveX uses multiple external DNS servers to resolve domain names to IP addresses, ensuring redundancy and reliability.

5. **Subdomain Discovery** [TODO]:  
   For wildcard domains, any newly queried subdomain will be resolved dynamically, and the corresponding IP addresses will be added to BGP announcements.

## How It Works

1. **Domain Addition**:  
   Users add a domain (or a domain with subdomains) through the admin panel.  
   Example:
    - Single Domain: `example.com`
    - [TODO] Domain with Subdomains: `*.example.com`

2. **DNS Resolution**:  
   The DNS service resolves the IP addresses for the domains using external DNS servers.

3. **BGP Announcement**:  
   After resolving the IP addresses, ResolveX announces them via the BGP service, ensuring the IPs are properly routed across the network.

## Setup

TBD (Provide detailed setup instructions here)

## Contributing

If you'd like to contribute to ResolveX, please submit a pull request or open an issue on the GitHub repository.

## License

ResolveX is licensed under the [MIT License](LICENSE).