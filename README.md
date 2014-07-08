goxmpp2
=======

Extensible library for handling the XMPP protocol (RFC 3920). This
code is inspired by, but not derived from,
https://github.com/mattn/go-xmpp/.

The core of the protocol is handled by xmpp.go, structs.go, and
stream.go. Everything else is an extension, though some of the
provided "extensions" are mandatory pieces of the protocol. Many of
the XEPs at http://xmpp.org/xmpp-protocols/xmpp-extensions/ can be
supported by this library, though at present only base protocol
support is here.

An simple client using this library is in the example directory. A
more interesting example can be found at
https://cjones.org/hg/foosfiend.

This software is written by Chris Jones <chris@cjones.org>. If you use
it, I'd appreciate a note letting me know. Bug reports are
welcome. The license is in LICENSE.txt; it's a BSD 2-Clause license
from http://opensource.org/licenses/BSD-2-Clause.
