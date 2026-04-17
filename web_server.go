package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func RunWebServer(ds *DataStore) {
	// 路由：查看完整内存快照
	http.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		ds.mu.RLock()
		defer ds.mu.RUnlock()

		snapshot := struct {
			Reg16Raw    []uint16  `json:"reg16_raw"`
			Reg16Scaled []float64 `json:"reg16_scaled"`
			Reg32Raw    []uint32  `json:"reg32_raw"`
			Reg32Scaled []float64 `json:"reg32_scaled"`
			Digitals    []bool    `json:"digitals"`
		}{
			Reg16Raw:    ds.RawReg16[:],
			Reg16Scaled: ds.Scaled16[:],
			Reg32Raw:    ds.RawReg32[:],
			Reg32Scaled: ds.Scaled32[:],
			Digitals:    ds.RawDigital[:],
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(snapshot)
	})

	// 路由：查看特定通道元数据
	http.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ds.Meta)
	})

	port := ":8080"
	log.Printf("🌐 Web 诊断台已启动: http://localhost%s/api/snapshot", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Web服务器启动失败: %v", err)
	}
}
