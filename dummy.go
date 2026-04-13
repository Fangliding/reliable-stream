package reliablestream

import (
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// noopDispatcher silently drops packets during shutdown.
type noopDispatcher struct{}

func (noopDispatcher) DeliverNetworkPacket(tcpip.NetworkProtocolNumber, *stack.PacketBuffer) {}
func (noopDispatcher) DeliverLinkPacket(tcpip.NetworkProtocolNumber, *stack.PacketBuffer)    {}

// transportEndpoint (client)

func (e *transportEndpoint) MaxHeaderLength() uint16                      { return 0 }
func (e *transportEndpoint) LinkAddress() tcpip.LinkAddress               { return "" }
func (e *transportEndpoint) SetLinkAddress(tcpip.LinkAddress)             {}
func (e *transportEndpoint) Capabilities() stack.LinkEndpointCapabilities { return 0 }
func (e *transportEndpoint) Wait()                                        {}
func (e *transportEndpoint) ARPHardwareType() header.ARPHardwareType      { return header.ARPHardwareNone }
func (e *transportEndpoint) AddHeader(*stack.PacketBuffer)                {}
func (e *transportEndpoint) ParseHeader(*stack.PacketBuffer) bool         { return true }
func (e *transportEndpoint) SetMTU(uint32)                                {}
func (e *transportEndpoint) SetOnCloseAction(func())                      {}

// serverEndpoint (server)

func (e *serverEndpoint) MaxHeaderLength() uint16                      { return 0 }
func (e *serverEndpoint) LinkAddress() tcpip.LinkAddress               { return "" }
func (e *serverEndpoint) SetLinkAddress(tcpip.LinkAddress)             {}
func (e *serverEndpoint) Capabilities() stack.LinkEndpointCapabilities { return 0 }
func (e *serverEndpoint) SetMTU(uint32)                                {}
func (e *serverEndpoint) Wait()                                        {}
func (e *serverEndpoint) ARPHardwareType() header.ARPHardwareType      { return header.ARPHardwareNone }
func (e *serverEndpoint) AddHeader(*stack.PacketBuffer)                {}
func (e *serverEndpoint) ParseHeader(*stack.PacketBuffer) bool         { return true }
func (e *serverEndpoint) SetOnCloseAction(func())                      {}
