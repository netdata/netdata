# interface_inbound_errors

## OS: FreeBSD

When a packet is received by your system, it can be processed in one of four ways:

- It can be passed as input to a higher-level protocol.

- It can encounter an error which is reported back to the source.

- It can be dropped due to an error.

- It can be forwarded to the next hop on its path to its destination.

There are mechanisms to identify packets with errors and verify the integrity of the packet such as the Cyclic
Redundancy Check (CRC), the Frame check sequence (FCS), the header checksum (IPv4), and length checks. The Netdata agent
monitors the number of inbound errors for a specific network interface in the last 10 minutes.


<details>
<summary>The life of a packet</summary>

The following list from "Design and Implementation of the FreeBSD Operating System, The, 2nd Edition" (McKusick,
Neville-Neil and Watson) [[1]](https://www.pearson.com/us/higher-education/program/Mc-Kusick-Design-and-Implementation-of-the-Free-BSD-Operating-System-The-2nd-Edition/PGM224032.html) 
provides a brief description of every action taken by your system for every packet it receives:

1. Verifies that the packet is at least as long as an IPv4 or IPv6 header and ensures that the header is contiguous.

2. For IPv4, checksums the header of the packet, and discards the packet if there is an error.

3. Verifies that the packet is at least as long as the header indicates, and drops the packet if it is not.

4. Does any filtering or security functions (ipfw, IPSec).

5. Processes any options associated with the header.

6. Checks whether the packet is for this host. If it is, continues processing the packet. If it is not, and if the
   system is acting as a router, your system will try to forward the packet. Otherwise, the packet is dropped.

7. If the packet has been fragmented, keeps it until all its fragments are received and reassembled, If the reassemble
   process takes a significant amount of time, the system drops it.

8. Passes the packet to the input routine of the next-higher-level protocol.

</details>

<details>
<summary>See more on CRC</summary>

> A cyclic redundancy check (CRC) is an error-detecting code commonly used in digital networks and storage devices to 
> detect accidental changes to raw data. Blocks of data entering these systems get a short check value attached, based 
> on the remainder of a polynomial division of their contents. On retrieval, the calculation is repeated and, in the 
> event the check values do not match, corrective action can be taken against data 
> corruption. [[2]](https://en.wikipedia.org/wiki/Cyclic_redundancy_check)

</details>

<details>
<summary>See more on FCS</summary>

> A frame check sequence (FCS) is an error-detecting code added to a frame in a communication protocol. All frames and 
> the bits, bytes, and fields contained within them, are susceptible to errors from a variety of sources. The FCS field 
> contains a number that is calculated by the source node based on the data in the frame. This number is added to the 
> end of a frame that is sent. When the destination node receives the frame the FCS number is recalculated and compared 
> with the FCS number included in the frame. If the two numbers are different, an error is assumed and the frame 
> is discarded. [[3]](https://en.wikipedia.org/wiki/Frame_check_sequence)

</details>

<details>
<summary>See more on header checksum</summary>

> The IPv4 header checksum is a checksum used in version 4 of the Internet Protocol (IPv4) to detect corruption in the 
> header of IPv4 packets. It is carried in the IP packet header and represents the 16-bit result of summation of the 
> header words. [[4]](https://en.wikipedia.org/wiki/IPv4_header_checksum)

</details>

<details>
<summary>References and sources</summary>

1. [Book: Design and Implementation of the FreeBSD Operating System (2nd-Edition)](https://www.pearson.com/us/higher-education/program/Mc-Kusick-Design-and-Implementation-of-the-Free-BSD-Operating-System-The-2nd-Edition/PGM224032.html)

1. [Cyclic redundancy check protocol](https://en.wikipedia.org/wiki/Cyclic_redundancy_check)

1. [Frame check sequence protocol](https://en.wikipedia.org/wiki/Frame_check_sequence)

1. [IPv4 header checksum protocol](https://en.wikipedia.org/wiki/IPv4_header_checksum)


</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

In any case, a good starting point is to get more information about the nature of your errors.

- Netdata dashboard provides an overview of these errors. You can see more in the `errors` chart under the IPv4 (or
  IPv6) section.

- `netstat` (network statistics) is a command-line network utility that displays network connections for Transmission
  Control Protocol, routing tables and network protocol statistics for any interface in your system.

    ```
    root@netdata~ # netstat -sI <your_interface>
    ```

</details>

<details>
<summary>Troubleshoot hardware errors in the link of the interface</summary>

You must identify which part of your topology causes these errors. Some actions you can take.

- Remove and re-install the optical fibers and optical modules and check whether the fiber connectors are damaged or
  contaminated. For ethernet interfaces check for damaged cables and/or for damages in the interfaces themselves.

  