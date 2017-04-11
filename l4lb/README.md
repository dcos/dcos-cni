# A CNI plugin for Minuteman and Spartan
Minuteman and Spartan are the internal distributed load balancer and
the distributed DNS service for DC/OS. This CNI plugin allows
containers running in an isolated virtual network, which don't allow
direct communication to the underlying host network, to get access to
services provides by Minuteman and Spartan.

You can get more details about the design and purpose the plugin in
this [design doc](https://goo.gl/xBUc71).
