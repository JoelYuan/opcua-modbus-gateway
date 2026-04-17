package main

import (
	"context"
	"log"
	"time"

	"github.com/gopcua/opcua/id"
	"github.com/gopcua/opcua/server"
	"github.com/gopcua/opcua/ua"
)

func RunOPCUAServer(ds *DataStore, mbMgr *ModbusManager) {
	host := "0.0.0.0"
	port := 4840
	s := server.New(
		server.EndPoint(host, port),
		server.EnableAuthMode(ua.UserTokenTypeAnonymous),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		log.Fatalf("❌ OPC UA 启动失败: %v", err)
	}
	defer s.Close()

	ns := server.NewNodeNameSpace(s, "ModbusNamespace")
	log.Printf("Node Namespace added at index %d", ns.ID())

	rootNs, _ := s.Namespace(0)
	rootObj := rootNs.Objects()
	nsObj := ns.Objects()
	rootObj.AddRef(nsObj, id.HasComponent, true)

	reg16Node := ns.AddNewVariableNode("Scaled16_Array", make([]float64, 1000))
	reg16Node.SetAttribute(ua.AttributeIDAccessLevel, server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead|ua.AccessLevelTypeCurrentWrite)))
	reg16Node.SetAttribute(ua.AttributeIDUserAccessLevel, server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead|ua.AccessLevelTypeCurrentWrite)))
	reg16Node.SetAttribute(ua.AttributeIDArrayDimensions, server.DataValueFromValue([]uint32{1000}))
	nsObj.AddRef(reg16Node, id.HasComponent, true)

	reg32Node := ns.AddNewVariableNode("Scaled32_Array", make([]float64, 500))
	reg32Node.SetAttribute(ua.AttributeIDAccessLevel, server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead|ua.AccessLevelTypeCurrentWrite)))
	reg32Node.SetAttribute(ua.AttributeIDUserAccessLevel, server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead|ua.AccessLevelTypeCurrentWrite)))
	reg32Node.SetAttribute(ua.AttributeIDArrayDimensions, server.DataValueFromValue([]uint32{500}))
	nsObj.AddRef(reg32Node, id.HasComponent, true)

	digNode := ns.AddNewVariableNode("Digital_Array", make([]bool, 3000))
	digNode.SetAttribute(ua.AttributeIDAccessLevel, server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead|ua.AccessLevelTypeCurrentWrite)))
	digNode.SetAttribute(ua.AttributeIDUserAccessLevel, server.DataValueFromValue(byte(ua.AccessLevelTypeCurrentRead|ua.AccessLevelTypeCurrentWrite)))
	digNode.SetAttribute(ua.AttributeIDArrayDimensions, server.DataValueFromValue([]uint32{3000}))
	nsObj.AddRef(digNode, id.HasComponent, true)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ds.mu.RLock()
			s16 := make([]float64, 1000)
			copy(s16, ds.Scaled16[:])
			s32 := make([]float64, 500)
			copy(s32, ds.Scaled32[:])
			sDig := make([]bool, 3000)
			copy(sDig, ds.ScaledDigital[:])
			ds.mu.RUnlock()

			val16 := ua.DataValue{
				Value:           ua.MustVariant(s16),
				SourceTimestamp: time.Now(),
				EncodingMask:    ua.DataValueValue | ua.DataValueSourceTimestamp,
			}
			reg16Node.SetAttribute(ua.AttributeIDValue, &val16)
			ns.ChangeNotification(reg16Node.ID())

			val32 := ua.DataValue{
				Value:           ua.MustVariant(s32),
				SourceTimestamp: time.Now(),
				EncodingMask:    ua.DataValueValue | ua.DataValueSourceTimestamp,
			}
			reg32Node.SetAttribute(ua.AttributeIDValue, &val32)
			ns.ChangeNotification(reg32Node.ID())

			valDig := ua.DataValue{
				Value:           ua.MustVariant(sDig),
				SourceTimestamp: time.Now(),
				EncodingMask:    ua.DataValueValue | ua.DataValueSourceTimestamp,
			}
			digNode.SetAttribute(ua.AttributeIDValue, &valDig)
			ns.ChangeNotification(digNode.ID())
		}
	}
}
