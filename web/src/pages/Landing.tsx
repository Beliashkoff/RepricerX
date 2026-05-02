import { PublicLayout } from '@/components/layout/PublicLayout'
import { Hero } from '@/components/landing/Hero'
import { Features } from '@/components/landing/Features'
import { HowItWorks } from '@/components/landing/HowItWorks'
import { StatsBlock } from '@/components/landing/StatsBlock'
import { FAQ } from '@/components/landing/FAQ'
import { Button } from '@/components/ui/button'
import { ArrowRight } from 'lucide-react'
import { useNavigate } from 'react-router-dom'

function CTA() {
  const navigate = useNavigate()
  return (
    <section className="py-20 bg-[#111] text-white">
      <div className="max-w-3xl mx-auto px-6 text-center">
        <h2 className="text-white mb-4">Начните управлять ценами уже сегодня</h2>
        <p className="text-white/60 text-lg mb-8">
          Подключите первый магазин за 2 минуты. Без сложных настроек.
        </p>
        <Button size="lg" onClick={() => navigate('/sign-up')} className="gap-2">
          Начать бесплатно <ArrowRight className="h-4 w-4" />
        </Button>
      </div>
    </section>
  )
}

export default function Landing() {
  return (
    <PublicLayout>
      <Hero />
      <StatsBlock />
      <Features />
      <HowItWorks />
      <FAQ />
      <CTA />
    </PublicLayout>
  )
}
