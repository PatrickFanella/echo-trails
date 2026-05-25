import SwiftUI

struct EchoAnnotationView: View {
    let echo: Echo

    var body: some View {
        VStack(spacing: 0) {
            ZStack {
                Circle()
                    .fill(Theme.amber.opacity(0.25))
                    .frame(width: 44, height: 44)

                Circle()
                    .fill(
                        RadialGradient(
                            colors: [Theme.amber, Theme.terracotta],
                            center: .center,
                            startRadius: 0,
                            endRadius: 16
                        )
                    )
                    .frame(width: 32, height: 32)

                Image(systemName: echo.type.icon)
                    .font(.system(size: 13, weight: .bold))
                    .foregroundStyle(.white)
            }

            Image(systemName: "triangle.fill")
                .font(.system(size: 8))
                .foregroundStyle(Theme.terracotta)
                .rotationEffect(.degrees(180))
                .offset(y: -2)
        }
    }
}
