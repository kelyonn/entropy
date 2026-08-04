import { useRef } from 'react';
import { Canvas, useFrame } from '@react-three/fiber';
import { OrbitControls } from '@react-three/drei';

function Server({ health, event }) {
  const meshRef = useRef();
  const particlesRef = useRef();
  const color =
    health === 'crashed' || health === 'error'
      ? '#b42318'
      : health === 'analyzing'
        ? '#a15c07'
        : '#1b7a4e';

  useFrame((state) => {
    if (!meshRef.current) return;
    const t = state.clock.elapsedTime;

    if (event === 'kernel_panic') {
      const scale = 1 + Math.sin(t * 40) * 0.15;
      meshRef.current.scale.set(scale, scale, scale);
      meshRef.current.rotation.x = t * 2;
      meshRef.current.rotation.y = t * 1.5;
    } else if (event === 'oom_kill') {
      meshRef.current.position.x = Math.sin(t * 28) * 0.12;
      meshRef.current.rotation.z = Math.sin(t * 22) * 0.06;
      meshRef.current.scale.set(1, 1, 1);
    } else if (event === 'segfault') {
      meshRef.current.position.y = Math.sin(t * 36) * 0.08;
      meshRef.current.rotation.y = Math.sin(t * 30) * 0.18;
      meshRef.current.scale.set(1, 0.85 + Math.abs(Math.sin(t * 18)) * 0.15, 1);
    } else if (health === 'crashed') {
      meshRef.current.position.x = Math.sin(t * 20) * 0.06;
      meshRef.current.rotation.z = Math.sin(t * 16) * 0.04;
      meshRef.current.scale.set(1, 1, 1);
    } else if (health === 'error') {
      meshRef.current.position.x = Math.sin(t * 12) * 0.03;
      meshRef.current.rotation.z = 0;
      meshRef.current.position.y = 0;
      meshRef.current.scale.set(1, 1, 1);
    } else {
      meshRef.current.position.y = Math.sin(t * 0.5) * 0.04;
      meshRef.current.position.x = 0;
      meshRef.current.rotation.z = 0;
      meshRef.current.scale.set(1, 1, 1);
    }

    if (particlesRef.current && event === 'kernel_panic') {
      particlesRef.current.rotation.y = t * 3;
      const burst = 0.4 + ((t * 1.8) % 1) * 1.6;
      particlesRef.current.scale.setScalar(burst);
    }
  });

  return (
    <>
      <mesh ref={meshRef}>
        <boxGeometry args={[1.4, 1.4, 1.4]} />
        <meshStandardMaterial
          color={color}
          emissive={color}
          emissiveIntensity={health === 'crashed' || event ? 0.5 : 0.12}
          wireframe={health === 'crashed' || event === 'oom_kill'}
        />
      </mesh>
      {event === 'kernel_panic' && (
        <group ref={particlesRef}>
          {[...Array(8)].map((_, i) => (
            <mesh key={i} position={[Math.cos(i) * 1.8, Math.sin(i * 0.7) * 1.2, Math.sin(i) * 1.8]}>
              <sphereGeometry args={[0.08, 8, 8]} />
              <meshStandardMaterial color="#b42318" emissive="#b42318" emissiveIntensity={0.8} />
            </mesh>
          ))}
        </group>
      )}
    </>
  );
}

export default function ServerViz({ health = 'healthy', event = null, cpu, memory }) {
  const label =
    health === 'crashed'
      ? 'Crashed'
      : health === 'analyzing'
        ? 'Analyzing'
        : health === 'error'
          ? 'Error'
          : 'Healthy';

  return (
    <div className="panel p-3">
      <div className="flex items-center justify-between mb-2">
        <h2 className="text-sm font-semibold text-ink">System view</h2>
        <span className="text-xs font-medium text-ink-muted">
          {event ? `${label} (${event})` : label}
        </span>
      </div>
      <div className="h-40 w-full rounded-panel bg-muted">
        <Canvas camera={{ position: [0, 0, 4] }}>
          <ambientLight intensity={0.55} />
          <pointLight position={[8, 10, 8]} intensity={0.9} />
          <Server health={health} event={event} />
          <OrbitControls enableZoom={false} autoRotate={health === 'healthy' && !event} autoRotateSpeed={0.8} />
        </Canvas>
      </div>
      {(cpu != null || memory != null) && (
        <div className="mt-2 flex gap-4 text-xs font-mono text-ink-muted">
          {cpu != null && <span>CPU {cpu}%</span>}
          {memory != null && <span>RAM {memory}%</span>}
        </div>
      )}
    </div>
  );
}
