package types

import (
	"fmt"
	"net"
	"sync"
)

type Addr string
type Id string

type Peer struct {
	Addr Addr
	Id   Id
	Conn *net.Conn
}

func (p Peer) String() string {
	return fmt.Sprintf("%v \t %v", p.Addr, p.Id)
}

type PeerById struct {
	sync.RWMutex
	Peers map[Id]*Peer
}

func (p *PeerById) Get(id Id) (peer *Peer, found bool) {
	p.RLock()
	defer p.RUnlock()

	peer, found = p.Peers[id]
	return
}

func (p *PeerById) Put(id Id, peer *Peer) {
	p.Lock()
	defer p.Unlock()

	p.Peers[id] = peer
}

func (p *PeerById) Del(id Id) {
	p.Lock()
	defer p.Unlock()

	delete(p.Peers, id)
}

type PeerByAddr struct {
	sync.RWMutex
	peers map[Addr]*Peer
}

func (p *PeerByAddr) Get(key Addr) (peer *Peer, found bool) {
	p.RLock()
	defer p.RUnlock()

	peer, found = p.peers[key]
	return
}

func (p *PeerByAddr) Put(key Addr, peer *Peer) {
	p.Lock()
	defer p.Unlock()

	p.peers[key] = peer
}

func (p *PeerByAddr) Del(addr Addr) {
	p.Lock()
	defer p.Unlock()

	delete(p.peers, addr)
}

type Peers struct {
	ByAddr PeerByAddr
	ById   PeerById
}

func (p *Peers) Add(conn *net.Conn, id Id) (peer *Peer) {
	addr := Addr((*conn).RemoteAddr().String())
	peer, found := p.ByAddr.Get(addr)
	if !found {
		peer = &Peer{addr, id, conn}
		p.ByAddr.Put(peer.Addr, peer)
		p.ById.Put(peer.Id, peer)
	} else {
		peer.Id = id
		peer.Conn = conn
	}

	return
}

func (p *Peers) Remove(conn net.Conn) {
	peer, b := p.ByAddr.Get(Addr(conn.RemoteAddr().String()))
	if !b {
		return
	}
	p.ById.Del(peer.Id)
	p.ByAddr.Del(peer.Addr)
}

func NewPeers() *Peers {
	return &Peers{
		ById: PeerById{
			Peers: make(map[Id]*Peer),
		},
		ByAddr: PeerByAddr{
			peers: make(map[Addr]*Peer),
		},
	}
}
