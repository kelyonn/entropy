import { useRef, useEffect } from 'react';
import { Canvas, useFrame } from '@react-three/fiber';
import { Box, Text, OrbitControls } from '@react-three/drei';

function Server({ crashed, analyzing }) {
  const meshRef = useRef();
  const color = crashed ? '#ef4444' : analyzing ? '#eab308' : '#22c55e';

  useFrame((state) => {
    if (crashed) {
      // Shake animation on crash
      meshRef.current.position.x = Math.sin(state.clock.elapsedTime * 30) * 0.1;
      meshRef.current.rotation.z = Math.sin(state.clock.elapsedTime * 25) * 0.05;
    } else {
      // Gentle float when healthy
      meshRef.current.position.y = Math.sin(state.clock.elapsedTime * 0.5) * 0.05;
      meshRef.current.position.x = 0;
      meshRef.current.rotation.z = 0;
    }
  });

  return (
    <mesh ref={meshRef}>
      <boxGeometry args={[1.5, 1.5, 1.5]}/>
      <meshStandardMaterial
        color={color}
        emissive={color}
        emissiveIntensity={crashed ? 0.8 : 0.2}
        wireframe={crashed}
      />
    </mesh>
  );
}

export default function ServerViz({ crashed, analyzing }) {
  return (
    <div className="h-48 w-full">
      <Canvas camera={{ position: [0, 0, 4] }}>
        <ambientLight intensity={0.5}/>
        <pointLight position={[10, 10, 10]} intensity={1}/>
        <Server crashed={crashed} analyzing={analyzing}/>
        <OrbitControls enableZoom={false} autoRotate={!crashed} autoRotateSpeed={1}/>
      </Canvas>
    </div>
  );
}
